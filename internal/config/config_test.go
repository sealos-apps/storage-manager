package config

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const validConfigYAML = `
viewer:
  backend_verify_url: http://backend/internal/filebrowser-hook/verify
  hook_client_token: hook-token
  hook_script: |
    #!/bin/sh
    echo hook.action=block
  filebrowser:
    image: filebrowser/filebrowser:v2.30.0
    binary_path: /filebrowser
    port: 8080
    token_ttl: 15m
    login_timeout: 2s
  pod:
    mount_path: /srv
    database_path: /tmp/filebrowser.db
    cpu_request: 50m
    memory_request: 64Mi
    cpu_limit: 500m
    memory_limit: 512Mi
  service:
    type: ClusterIP
    port: 80
  ingress:
    class_name: nginx
    host_template: viewer-{{ .PodSessionID }}.example.test
`

type deployRule struct {
	APIGroup  []string `yaml:"apiGroups"`
	Resources []string `yaml:"resources"`
	Verbs     []string `yaml:"verbs"`
}

func TestLoadAppliesDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load([]byte(`
viewer:
  backend_verify_url: http://backend/internal/filebrowser-hook/verify
  hook_client_token: super-secret
  hook_script: |
    #!/bin/sh
    echo hook.action=block
  filebrowser:
    image: custom/filebrowser:v1
    binary_path: /custom-filebrowser
    port: 8081
    token_ttl: 20m
    login_timeout: 3s
  pod:
    mount_path: /data
    database_path: /tmp/fb.db
    cpu_request: 100m
    memory_request: 128Mi
    cpu_limit: 1
    memory_limit: 1Gi
  service:
    type: ClusterIP
    port: 8080
  ingress:
    class_name: nginx
    host_template: viewer-{{ .PodSessionID }}.example.test
observability:
  logs:
    level: debug
  traces:
    exporter: otlp
    endpoint: http://otel-collector:4318/v1/traces
    sample_ratio: 0.25
    batch_timeout: 3s
    export_timeout: 2s
debug:
  enabled: true
  management_kubeconfig_path: config/kubeconfig.management.yaml
  forced_namespace: ns-debug
admin:
  allowed_user_ids:
    - admin
  storage_class_service_account_name: storageclass-admin
`))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Viewer.FileBrowser.Image != "custom/filebrowser:v1" {
		t.Fatalf("image = %q", cfg.Viewer.FileBrowser.Image)
	}
	if cfg.Viewer.FileBrowser.BinaryPath != "/custom-filebrowser" {
		t.Fatalf("binary path = %q", cfg.Viewer.FileBrowser.BinaryPath)
	}
	if cfg.Viewer.FileBrowser.TokenTTL != 20*time.Minute {
		t.Fatalf("token ttl = %s", cfg.Viewer.FileBrowser.TokenTTL)
	}
	if !cfg.Debug.Enabled {
		t.Fatal("debug.enabled = false")
	}
	if cfg.Debug.ManagementKubeconfigPath != "config/kubeconfig.management.yaml" {
		t.Fatalf("debug management kubeconfig path = %q", cfg.Debug.ManagementKubeconfigPath)
	}
	if cfg.Debug.ForcedNamespace != "ns-debug" {
		t.Fatalf("debug forced namespace = %q", cfg.Debug.ForcedNamespace)
	}
	if !slices.Contains(cfg.Admin.AllowedUserIDs, "admin") {
		t.Fatalf("admin allowed user ids = %#v", cfg.Admin.AllowedUserIDs)
	}
	if cfg.Admin.StorageClassServiceAccountName != "storageclass-admin" {
		t.Fatalf("admin storageclass service account name = %q", cfg.Admin.StorageClassServiceAccountName)
	}
	if cfg.Observability.Logs.Level != "debug" {
		t.Fatalf("log level = %q", cfg.Observability.Logs.Level)
	}
	if cfg.Observability.Traces.Exporter != "otlp" {
		t.Fatalf("trace exporter = %q", cfg.Observability.Traces.Exporter)
	}
	if cfg.Observability.Traces.Endpoint != "http://otel-collector:4318/v1/traces" {
		t.Fatalf("trace endpoint = %q", cfg.Observability.Traces.Endpoint)
	}
	if cfg.Observability.Traces.SampleRatio != 0.25 {
		t.Fatalf("trace sample ratio = %f", cfg.Observability.Traces.SampleRatio)
	}
	if cfg.Observability.Traces.BatchTimeout != 3*time.Second {
		t.Fatalf("trace batch timeout = %s", cfg.Observability.Traces.BatchTimeout)
	}
	if cfg.Observability.Traces.ExportTimeout != 2*time.Second {
		t.Fatalf("trace export timeout = %s", cfg.Observability.Traces.ExportTimeout)
	}
}

func TestDefaultOmitsDeploymentValues(t *testing.T) {
	t.Parallel()

	cfg := Default()
	if cfg.Viewer.BackendVerifyURL != "" ||
		cfg.Viewer.HookClientToken != "" ||
		cfg.Viewer.HookScript != "" ||
		cfg.Viewer.FileBrowser.Image != "" ||
		cfg.Viewer.FileBrowser.BinaryPath != "" ||
		cfg.Viewer.FileBrowser.Port != 0 ||
		cfg.Viewer.Ingress.ClassName != "" ||
		cfg.Viewer.Ingress.HostTemplate != "" {
		t.Fatalf("Default() contains deployment values: %#v", cfg.Viewer)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing deployment fields",
			body: "",
			want: "viewer.backend_verify_url is required",
		},
		{
			name: "missing hook script",
			body: replaceConfig(t, validConfigYAML, "  hook_script: |\n    #!/bin/sh\n    echo hook.action=block\n", "  hook_script: \"\"\n"),
			want: "viewer.hook_script is required",
		},
		{
			name: "missing binary path",
			body: replaceConfig(t, validConfigYAML, "    binary_path: /filebrowser\n", "    binary_path: \"\"\n"),
			want: "viewer.filebrowser.binary_path is required",
		},
		{
			name: "database inside pvc mount",
			body: replaceConfig(t, validConfigYAML, "    database_path: /tmp/filebrowser.db\n", "    database_path: /srv/filebrowser.db\n"),
			want: "database_path must not be inside",
		},
		{
			name: "bad log exporter",
			body: validConfigYAML + "\nobservability:\n  logs:\n    exporter: otlp\n",
			want: "logs.exporter",
		},
		{
			name: "otlp traces without endpoint",
			body: validConfigYAML + "\nobservability:\n  traces:\n    exporter: otlp\n",
			want: "traces.endpoint",
		},
		{
			name: "bad trace sample ratio",
			body: validConfigYAML + "\nobservability:\n  traces:\n    exporter: none\n    sample_ratio: 1.1\n",
			want: "traces.sample_ratio",
		},
		{
			name: "empty admin user id",
			body: validConfigYAML + "\nadmin:\n  allowed_user_ids:\n    - \"\"\n",
			want: "admin.allowed_user_ids",
		},
		{
			name: "bad storageclass service account name",
			body: validConfigYAML + "\nadmin:\n  storage_class_service_account_name: BAD_NAME\n",
			want: "admin.storage_class_service_account_name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load([]byte(tt.body))
			if err == nil {
				t.Fatal("Load() error = nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tt.want)
			}
		})
	}
}

func TestRedactedHidesHookToken(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Viewer.HookClientToken = "secret"
	cfg.Viewer.HookScript = "secret script"
	redacted := cfg.Redacted()
	viewer, ok := redacted["viewer"].(map[string]any)
	if !ok {
		t.Fatalf("viewer redaction has type %T", redacted["viewer"])
	}
	if viewer["hook_client_token"] == "secret" {
		t.Fatal("hook token leaked in redacted output")
	}
	if viewer["hook_script"] == "secret script" {
		t.Fatal("hook script leaked in redacted output")
	}
}

func TestLoadFileUsesExplicitPathBeforeEnv(t *testing.T) {
	dir := t.TempDir()
	explicitPath := filepath.Join(dir, "explicit.yaml")
	envPath := filepath.Join(dir, "env.yaml")
	if err := os.WriteFile(explicitPath, []byte(validConfigYAML), 0o600); err != nil {
		t.Fatalf("write explicit config: %v", err)
	}
	if err := os.WriteFile(envPath, []byte(replaceConfig(t, validConfigYAML, "filebrowser/filebrowser:v2.30.0", "env/filebrowser:v1")), 0o600); err != nil {
		t.Fatalf("write env config: %v", err)
	}
	t.Setenv(EnvPath, envPath)

	cfg, err := LoadFile(explicitPath)
	if err != nil {
		t.Fatalf("LoadFile(explicit) error = %v", err)
	}
	if cfg.Server.ConfigPath != explicitPath {
		t.Fatalf("config path = %q", cfg.Server.ConfigPath)
	}
	if cfg.Viewer.FileBrowser.Image != "filebrowser/filebrowser:v2.30.0" {
		t.Fatalf("image = %q", cfg.Viewer.FileBrowser.Image)
	}
}

func TestLoadFileUsesEnvPathWhenPathEmpty(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env.yaml")
	if err := os.WriteFile(envPath, []byte(validConfigYAML), 0o600); err != nil {
		t.Fatalf("write env config: %v", err)
	}
	t.Setenv(EnvPath, envPath)

	cfg, err := LoadFile("")
	if err != nil {
		t.Fatalf("LoadFile(\"\") error = %v", err)
	}
	if cfg.Server.ConfigPath != envPath {
		t.Fatalf("config path = %q", cfg.Server.ConfigPath)
	}
}

func TestLoadDoesNotReadEnvPath(t *testing.T) {
	t.Setenv(EnvPath, filepath.Join(t.TempDir(), "missing.yaml"))
	cfg, err := Load([]byte(validConfigYAML))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.ConfigPath != DefaultPath {
		t.Fatalf("config path = %q", cfg.Server.ConfigPath)
	}
}

func TestCommittedExampleConfigLoads(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	if _, err := LoadFile(filepath.Join(root, "config", "viewer.example.yaml")); err != nil {
		t.Fatalf("LoadFile(viewer.example.yaml) error = %v", err)
	}
}

func TestCommittedDebugExampleConfigLoads(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	cfg, err := LoadFile(filepath.Join(root, "config", "viewer.debug.example.yaml"))
	if err != nil {
		t.Fatalf("LoadFile(viewer.debug.example.yaml) error = %v", err)
	}
	if !cfg.Debug.Enabled {
		t.Fatal("debug example must enable debug")
	}
	if strings.TrimSpace(cfg.Debug.ManagementKubeconfigPath) == "" {
		t.Fatal("debug example missing management kubeconfig path")
	}
}

func TestCommittedIntegrationExampleConfigLoads(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	cfg, err := LoadFile(filepath.Join(root, "config", "viewer.integration.example.yaml"))
	if err != nil {
		t.Fatalf("LoadFile(viewer.integration.example.yaml) error = %v", err)
	}
	if !cfg.Debug.Enabled {
		t.Fatal("integration example must enable debug")
	}
	if strings.TrimSpace(cfg.Debug.ManagementKubeconfigPath) == "" {
		t.Fatal("integration example missing management kubeconfig path")
	}
}

func TestCommittedHookScriptAcceptsSpacedJSON(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	cfg, err := LoadFile(filepath.Join(root, "config", "viewer.example.yaml"))
	if err != nil {
		t.Fatalf("LoadFile(viewer.example.yaml) error = %v", err)
	}
	scriptPath := filepath.Join(t.TempDir(), "filebrowser-auth-hook.sh")
	if err := os.WriteFile(scriptPath, []byte(cfg.Viewer.HookScript), 0o600); err != nil {
		t.Fatalf("write hook script: %v", err)
	}
	if err := os.Chmod(scriptPath, 0o700); err != nil { //nolint:gosec // Test executes a temporary hook script.
		t.Fatalf("chmod hook script: %v", err)
	}
	verify := responseServer(t, `{
  "filebrowser_hook_verification": {
    "allow": true,
    "permissions": {
      "create": true
    }
  }
}`)

	output, err := runHookScript(t, scriptPath, map[string]string{
		"PASSWORD":           "ar_1234567890abcdef.secret",
		"USERNAME":           "vs_1234567890abcdef",
		"POD_SESSION_ID":     "ps_1234567890abcdef",
		"VIEWER_POD_NAME":    "viewer-ps-1234567890abcdef",
		"HOOK_CLIENT_TOKEN":  "hook-token",
		"BACKEND_VERIFY_URL": verify.URL,
	})
	if err != nil {
		t.Fatalf("hook script error = %v output=%s", err, output)
	}
	if !strings.Contains(output, "hook.action=auth") {
		t.Fatalf("output = %s", output)
	}
	if !strings.Contains(output, "user.hideDotfiles=false") {
		t.Fatalf("output = %s", output)
	}
	if !strings.Contains(output, "user.perm.create=true") {
		t.Fatalf("output = %s", output)
	}
}

func TestDeployChartValuesEmbedValidViewerConfig(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "deploy", "values.yaml")) //nolint:gosec // Test reads a committed fixture.
	if err != nil {
		t.Fatalf("read deploy values: %v", err)
	}
	var values struct {
		Backend struct {
			Config struct {
				ViewerYAML string `yaml:"viewerYaml"`
			} `yaml:"config"`
		} `yaml:"backend"`
	}
	if err := yaml.Unmarshal(data, &values); err != nil {
		t.Fatalf("parse deploy values: %v", err)
	}
	viewerYAML := values.Backend.Config.ViewerYAML
	if strings.TrimSpace(viewerYAML) == "" {
		t.Fatal("deploy values missing backend.config.viewerYaml")
	}
	if _, err := Load([]byte(viewerYAML)); err != nil {
		t.Fatalf("embedded deploy viewer.yaml error = %v", err)
	}
	for _, forbidden := range []string{
		"namespace_allowlist",
		"management_kubeconfig_path",
		"storage_class_name",
	} {
		if strings.Contains(viewerYAML, forbidden) {
			t.Fatalf("deploy config contains %q", forbidden)
		}
	}
}

func TestDeployServiceAccountAllowsViewerPodCleanup(t *testing.T) {
	t.Parallel()

	data := renderDeployChart(t)
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	var clusterRole struct {
		Kind  string       `yaml:"kind"`
		Rules []deployRule `yaml:"rules"`
	}
	for {
		var document struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Rules []deployRule `yaml:"rules"`
		}
		if err := decoder.Decode(&document); err != nil {
			if err != io.EOF {
				t.Fatalf("parse rendered deploy chart: %v", err)
			}
			break
		}
		if document.Kind == "ClusterRole" && document.Metadata.Name == "viewer-backend" {
			clusterRole.Kind = document.Kind
			clusterRole.Rules = document.Rules
			break
		}
	}
	if clusterRole.Kind != "ClusterRole" {
		t.Fatal("deploy service account missing ClusterRole")
	}
	requireRule(t, clusterRole.Rules, "", "pods", []string{"get", "list", "delete", "patch"})
	requireRule(t, clusterRole.Rules, "", "services", []string{"get", "list", "delete"})
	requireRule(t, clusterRole.Rules, "", "configmaps", []string{"get", "list", "delete"})
	requireRule(t, clusterRole.Rules, "networking.k8s.io", "ingresses", []string{"get", "list", "delete"})
}

func TestDeployStorageClassAdminManifest(t *testing.T) {
	t.Parallel()

	data := renderDeployChart(t)
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	foundSA := false
	foundStorageRole := false
	foundImpersonateRole := false
	for {
		var document struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name      string `yaml:"name"`
				Namespace string `yaml:"namespace"`
			} `yaml:"metadata"`
			Rules []deployRule `yaml:"rules"`
		}
		if err := decoder.Decode(&document); err != nil {
			if err != io.EOF {
				t.Fatalf("parse deploy storageclass admin manifest: %v", err)
			}
			break
		}
		if document.Kind == "ServiceAccount" &&
			document.Metadata.Name == "storageclass-admin" &&
			document.Metadata.Namespace == "sealos-storage-manager" {
			foundSA = true
		}
		if document.Kind == "ClusterRole" && document.Metadata.Name == "storageclass-admin" {
			requireRule(t, document.Rules, "storage.k8s.io", "storageclasses", []string{"get", "list", "create", "update", "delete"})
			foundStorageRole = true
		}
		if document.Kind == "Role" && document.Metadata.Name == "viewer-backend-impersonate-storageclass-admin" {
			requireRule(t, document.Rules, "", "serviceaccounts", []string{"impersonate"})
			foundImpersonateRole = true
		}
	}
	if !foundSA || !foundStorageRole || !foundImpersonateRole {
		t.Fatalf("manifest found serviceAccount=%v storageRole=%v impersonateRole=%v", foundSA, foundStorageRole, foundImpersonateRole)
	}
}

func renderDeployChart(t *testing.T) []byte {
	t.Helper()

	if _, err := exec.LookPath("helm"); err != nil {
		t.Skipf("helm not installed: %v", err)
	}
	root := repoRoot(t)
	cmd := exec.Command("helm", "template", "sealos-storage-manager", filepath.Join(root, "deploy"), "--namespace", "sealos-storage-manager") //nolint:gosec // Test renders committed chart path.
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("render deploy chart: %v\n%s", err, output)
	}
	return output
}

func requireRule(t *testing.T, rules []deployRule, apiGroup string, resource string, verbs []string) {
	t.Helper()

	for _, rule := range rules {
		if !containsString(rule.APIGroup, apiGroup) || !containsString(rule.Resources, resource) {
			continue
		}
		for _, verb := range verbs {
			if !containsString(rule.Verbs, verb) {
				t.Fatalf("rule for %s/%s missing verb %q: %v", apiGroup, resource, verb, rule.Verbs)
			}
		}
		return
	}
	t.Fatalf("missing rule for %s/%s", apiGroup, resource)
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

func responseServer(t *testing.T, body string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func runHookScript(t *testing.T, scriptPath string, env map[string]string) (string, error) {
	t.Helper()

	cmd := exec.Command("/bin/sh", scriptPath) //nolint:gosec // Test executes a committed hook script fixture.
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func replaceConfig(t *testing.T, body string, old string, replacement string) string {
	t.Helper()

	updated := strings.Replace(body, old, replacement, 1)
	if updated == body {
		t.Fatalf("config replacement did not match %q", old)
	}
	return updated
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root")
		}
		dir = parent
	}
}
