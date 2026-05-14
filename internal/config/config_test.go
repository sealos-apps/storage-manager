package config

import (
	"os"
	"path/filepath"
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

func TestLoadAppliesDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load([]byte(`
kubernetes:
  management_kubeconfig_path: kubeconfig.test.yaml
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
	if cfg.Kubernetes.ManagementKubeconfigPath != "kubeconfig.test.yaml" {
		t.Fatalf("management kubeconfig path = %q", cfg.Kubernetes.ManagementKubeconfigPath)
	}
	if cfg.Viewer.FileBrowser.TokenTTL != 20*time.Minute {
		t.Fatalf("token ttl = %s", cfg.Viewer.FileBrowser.TokenTTL)
	}
	if cfg.Observability.Logs.Level != "debug" {
		t.Fatalf("log level = %q", cfg.Observability.Logs.Level)
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

func TestCommittedExampleConfigLoads(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	if _, err := LoadFile(filepath.Join(root, "config", "viewer.example.yaml")); err != nil {
		t.Fatalf("LoadFile(viewer.example.yaml) error = %v", err)
	}
}

func TestDeployConfigMapEmbedsValidViewerConfig(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "deploy", "configmap.yaml")) //nolint:gosec // Test reads a committed fixture.
	if err != nil {
		t.Fatalf("read deploy configmap: %v", err)
	}
	var manifest struct {
		Data map[string]string `yaml:"data"`
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse deploy configmap: %v", err)
	}
	viewerYAML := manifest.Data["viewer.yaml"]
	if strings.TrimSpace(viewerYAML) == "" {
		t.Fatal("deploy configmap missing viewer.yaml")
	}
	if _, err := Load([]byte(viewerYAML)); err != nil {
		t.Fatalf("embedded viewer.yaml error = %v", err)
	}
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
