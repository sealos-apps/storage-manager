package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var dns1123Invalid = regexp.MustCompile(`[^a-z0-9-]+`)

func (s *PodService) viewerURL(id string) (string, error) {
	host, err := s.viewerHost(id)
	if err != nil {
		return "", err
	}
	scheme := "https"
	if s.cfg.Viewer.Ingress.TLSSecretName == "" {
		scheme = "http"
	}
	return scheme + "://" + host, nil
}

func (s *PodService) internalViewerURL(namespace string, serviceName string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", serviceName, namespace, s.cfg.Viewer.Service.Port)
}

func (s *PodService) viewerHost(id string) (string, error) {
	tmpl, err := template.New("host").Parse(s.cfg.Viewer.Ingress.HostTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing viewer host template: %w", err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, map[string]string{"PodSessionID": dnsLabel(id)}); err != nil {
		return "", fmt.Errorf("executing viewer host template: %w", err)
	}
	return out.String(), nil
}

func resourceName(name string) string {
	name = dnsLabel(name)
	if len(name) <= 63 {
		return name
	}
	return name[:63]
}

func dnsLabel(value string) string {
	value = strings.ToLower(value)
	value = dns1123Invalid.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "viewer"
	}
	return value
}

func resourceList(cpu string, memory string) corev1.ResourceList {
	resources := corev1.ResourceList{}
	if cpu != "" {
		resources[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		resources[corev1.ResourceMemory] = resource.MustParse(memory)
	}
	return resources
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func ptrInt32(value int32) *int32 {
	return ptr(value)
}

func ptr[T any](value T) *T {
	return &value
}

func hookConfigMapName(session *domain.PodSession) string {
	return session.PodName
}

func podOwnerReference(pod *corev1.Pod) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       pod.Name,
		UID:        pod.UID,
	}
}

func managedLabels(session *domain.PodSession) map[string]string {
	return map[string]string{
		labelComponent:      componentViewer,
		labelPVCName:        session.PVCName,
		labelPVCUID:         session.PVCUID,
		labelPodSessionID:   session.ID,
		labelRuntimeVersion: session.RuntimeVersion,
	}
}

func lifecycleAnnotations(session *domain.PodSession) map[string]string {
	annotations := map[string]string{
		annotationAccessMode:     session.AccessMode,
		annotationCreatedAt:      session.CreatedAt.Format(time.RFC3339Nano),
		annotationKeepaliveUntil: session.ExpiresAt.Format(time.RFC3339Nano),
		annotationLastActiveAt:   session.LastActiveAt.Format(time.RFC3339Nano),
		annotationMode:           session.Mode,
		annotationRuntimeVersion: session.RuntimeVersion,
	}
	return annotations
}

func keepaliveAnnotations(session *domain.PodSession) map[string]string {
	return map[string]string{
		annotationKeepaliveUntil: session.ExpiresAt.Format(time.RFC3339Nano),
		annotationLastActiveAt:   session.LastActiveAt.Format(time.RFC3339Nano),
	}
}

func parseAnnotationTime(annotations map[string]string, key string) (time.Time, bool) {
	value := annotations[key]
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func runtimeVersion(cfg config.Config) string {
	type versionedViewerConfig struct {
		BackendVerifyURL    string                   `json:"backend_verify_url"`
		HookClientTokenHash string                   `json:"hook_client_token_hash"`
		HookScript          string                   `json:"hook_script"`
		FileBrowser         config.FileBrowserConfig `json:"filebrowser"`
		Pod                 config.PodConfig         `json:"pod"`
		Service             config.ServiceConfig     `json:"service"`
		Ingress             config.IngressConfig     `json:"ingress"`
	}
	tokenHash := sha256.Sum256([]byte(cfg.Viewer.HookClientToken))
	body, err := json.Marshal(versionedViewerConfig{
		BackendVerifyURL:    cfg.Viewer.BackendVerifyURL,
		HookClientTokenHash: hex.EncodeToString(tokenHash[:]),
		HookScript:          cfg.Viewer.HookScript,
		FileBrowser:         cfg.Viewer.FileBrowser,
		Pod:                 cfg.Viewer.Pod,
		Service:             cfg.Viewer.Service,
		Ingress:             cfg.Viewer.Ingress,
	})
	if err != nil {
		panic(fmt.Sprintf("marshaling runtime version config: %v", err))
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])[:12]
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
