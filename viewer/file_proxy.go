package viewer

import (
	"io"
	"net/http"
	"net/url"
	pathpkg "path"
	"strconv"
	"strings"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/authn"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
)

const (
	fileProxyResources = "resources"
	fileProxyRecursive = "recursive"
	fileProxyUsage     = "usage"
	fileProxyRaw       = "raw"
	fileProxyTUS       = "tus"
	viewerSessionQuery = "viewer_session_id"
	filePathQuery      = "path"
	overrideQuery      = "override"
	destinationQuery   = "destination"
	actionQuery        = "action"
	tusUploadPathQuery = "upload_path"
)

// ProxyViewerFiles is the shared raw data-plane handler for the explicitly
// listed file operations declared in api.go. It never accepts a target URL
// from the client and never returns the File Browser credential.
func (h *Handler) ProxyViewerFiles(w http.ResponseWriter, req *http.Request, operation string) {
	start := time.Now()
	route := "/viewer-files/" + operation
	if apiErr := h.requireFileManagementEnabled(); apiErr != nil {
		h.observe(req.Context(), req.Method, route, apiErr.Status, start)
		apienv.WriteError(w, apiErr)
		return
	}

	viewerSessionID := strings.TrimSpace(req.URL.Query().Get(viewerSessionQuery))
	if viewerSessionID == "" {
		apiErr := apienv.NewError(http.StatusBadRequest, apienv.CodeValidationError, "viewer_session_id is required", nil)
		h.observe(req.Context(), req.Method, route, apiErr.Status, start)
		apienv.WriteError(w, apiErr)
		return
	}
	filePath, apiErr := proxyPath(req.URL.Query().Get(filePathQuery))
	if apiErr != nil {
		h.observe(req.Context(), req.Method, route, apiErr.Status, start)
		apienv.WriteError(w, apiErr)
		return
	}

	principal, authErr := h.authenticateRequest(&AuthenticatedRequest{Authorization: req.Header.Get("Authorization")})
	if authErr != nil {
		h.observe(req.Context(), req.Method, route, authErr.Status, start)
		apienv.WriteError(w, authErr)
		return
	}
	ctx := authn.WithPrincipal(req.Context(), principal)
	if err := h.authorizeViewerSessionPVC(ctx, principal, viewerSessionID); err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, req.Method, route, apiErr.Status, start)
		apienv.WriteError(w, apiErr)
		return
	}

	token, err := h.viewers.IssueToken(ctx, viewerSessionID, principal.ID)
	if err != nil {
		apiErr := apienv.FromError(err)
		h.observe(ctx, req.Method, route, apiErr.Status, start)
		apienv.WriteError(w, apiErr)
		return
	}
	targetURL, apiErr := h.fileBrowserTarget(token, operation, filePath, req)
	if apiErr != nil {
		h.observe(ctx, req.Method, route, apiErr.Status, start)
		apienv.WriteError(w, apiErr)
		return
	}
	response, proxyErr := h.fileBrowserProxy.Proxy(ctx, targetURL, req, token.Token)
	if proxyErr != nil {
		apiErr := apienv.NewError(http.StatusBadGateway, apienv.CodeFileBrowserLoginFailed, "File Browser request failed", nil)
		h.observe(ctx, req.Method, route, apiErr.Status, start)
		apienv.WriteError(w, apiErr)
		return
	}
	defer func() { _ = response.Body.Close() }()
	writeFileProxyResponse(w, req, response, operation, viewerSessionID, filePath)
	h.observe(ctx, req.Method, route, response.StatusCode, start)
}

func (h *Handler) fileBrowserTarget(token *domain.ViewerToken, operation string, filePath string, req *http.Request) (string, *apienv.Error) {
	baseURL := token.InternalViewerURL
	if baseURL == "" {
		baseURL = token.ViewerURL
	}
	endpoint := "/api/" + operation
	switch operation {
	case fileProxyResources, fileProxyRecursive, fileProxyUsage, fileProxyRaw, fileProxyTUS:
	default:
		return "", apienv.NewError(http.StatusBadRequest, apienv.CodeValidationError, "unsupported file operation", nil)
	}
	if operation == fileProxyRecursive {
		endpoint = "/api/resources/recursive"
	}
	if operation == fileProxyResources {
		endpoint = "/api/resources"
	}
	if operation == fileProxyRaw {
		endpoint = "/api/raw"
	}
	if operation == fileProxyTUS {
		endpoint = "/api/tus"
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", apienv.NewError(http.StatusBadGateway, apienv.CodeInternal, "File Browser URL is invalid", nil)
	}
	if operation == fileProxyTUS {
		if uploadPath := strings.TrimSpace(req.URL.Query().Get(tusUploadPathQuery)); uploadPath != "" {
			if !validTUSUploadPath(uploadPath) {
				return "", apienv.NewError(http.StatusBadRequest, apienv.CodeValidationError, "invalid TUS upload path", nil)
			}
			endpoint = uploadPath
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + endpoint
	if req.URL.Query().Get(tusUploadPathQuery) == "" {
		if filePath == "/" {
			parsed.Path += "/"
		} else {
			parsed.Path += filePath
		}
	}
	parsed.RawPath = ""
	query := parsed.Query()
	if operation == fileProxyResources || (operation == fileProxyTUS && req.URL.Query().Get(tusUploadPathQuery) == "") {
		query.Set(overrideQuery, strconv.FormatBool(queryBool(req.URL.Query().Get(overrideQuery))))
	}
	if operation == fileProxyRaw && queryBool(req.URL.Query().Get("inline")) {
		query.Set("inline", "true")
	}
	if operation == fileProxyResources && req.Method == http.MethodPatch {
		action := req.URL.Query().Get(actionQuery)
		if action != "rename" && action != "copy" {
			return "", apienv.NewError(http.StatusBadRequest, apienv.CodeValidationError, "unsupported file action", nil)
		}
		query.Set(actionQuery, action)
		destination, destinationErr := proxyPath(req.URL.Query().Get(destinationQuery))
		if destinationErr != nil {
			return "", destinationErr
		}
		query.Set(destinationQuery, destination)
		query.Set("rename", strconv.FormatBool(action == "rename"))
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func writeFileProxyResponse(
	w http.ResponseWriter,
	req *http.Request,
	response *http.Response,
	operation string,
	viewerSessionID string,
	filePath string,
) {
	for key, values := range response.Header {
		if strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "X-Auth") || strings.EqualFold(key, "Set-Cookie") || strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Location") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if location := response.Header.Get("Location"); location != "" && operation == fileProxyTUS {
		uploadPath, ok := tusProxyUploadPath(location)
		if !ok {
			w.WriteHeader(response.StatusCode)
			if req.Method != http.MethodHead {
				_, _ = io.Copy(w, response.Body)
			}
			return
		}
		query := req.URL.Query()
		query.Set(viewerSessionQuery, viewerSessionID)
		query.Set(filePathQuery, filePath)
		query.Set(tusUploadPathQuery, uploadPath)
		proxyLocation := *req.URL
		proxyLocation.RawQuery = query.Encode()
		w.Header().Set("Location", proxyLocation.RequestURI())
	}
	w.WriteHeader(response.StatusCode)
	if req.Method != http.MethodHead {
		_, _ = io.Copy(w, response.Body)
	}
}

func tusProxyUploadPath(rawLocation string) (string, bool) {
	parsed, err := url.Parse(rawLocation)
	if err != nil || !validTUSUploadPath(parsed.Path) {
		return "", false
	}
	return parsed.Path, true
}

func validTUSUploadPath(uploadPath string) bool {
	return uploadPath != "" && pathpkg.Clean(uploadPath) == uploadPath && strings.HasPrefix(uploadPath, "/api/tus/")
}

func proxyPath(raw string) (string, *apienv.Error) {
	if strings.TrimSpace(raw) == "" {
		return "/", nil
	}
	if !strings.HasPrefix(raw, "/") {
		return "", apienv.NewError(http.StatusBadRequest, apienv.CodeValidationError, "file path must start with /", nil)
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == ".." {
			return "", apienv.NewError(http.StatusBadRequest, apienv.CodeValidationError, "file path cannot escape the viewer root", nil)
		}
	}
	clean := pathpkg.Clean(raw)
	if clean == "." {
		return "/", nil
	}
	result := "/" + strings.TrimPrefix(clean, "/")
	if result != "/" && strings.HasSuffix(raw, "/") {
		result += "/"
	}
	return result, nil
}

func queryBool(value string) bool {
	return value == "1" || strings.EqualFold(value, "true")
}
