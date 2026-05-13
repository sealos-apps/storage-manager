package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadAppliesDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load([]byte(`
viewer:
  filebrowser:
    image: custom/filebrowser:v1
  hook_client_token: super-secret
observability:
  trace_sample_ratio: 0.5
`))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Viewer.FileBrowser.Image != "custom/filebrowser:v1" {
		t.Fatalf("image = %q", cfg.Viewer.FileBrowser.Image)
	}
	if cfg.Viewer.FileBrowser.TokenTTL != 15*time.Minute {
		t.Fatalf("token ttl = %s", cfg.Viewer.FileBrowser.TokenTTL)
	}
	if cfg.Observability.TraceSampleRatio != 0.5 {
		t.Fatalf("trace sample ratio = %f", cfg.Observability.TraceSampleRatio)
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
			name: "missing image",
			body: "viewer:\n  filebrowser:\n    image: \"\"\n",
			want: "viewer.filebrowser.image is required",
		},
		{
			name: "database inside pvc mount",
			body: "viewer:\n  pod:\n    mount_path: /srv\n    database_path: /srv/filebrowser.db\n",
			want: "database_path must not be inside",
		},
		{
			name: "bad trace ratio",
			body: "observability:\n  trace_sample_ratio: 2\n",
			want: "trace_sample_ratio",
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
	redacted := cfg.Redacted()
	viewer, ok := redacted["viewer"].(map[string]any)
	if !ok {
		t.Fatalf("viewer redaction has type %T", redacted["viewer"])
	}
	if viewer["hook_client_token"] == "secret" {
		t.Fatal("hook token leaked in redacted output")
	}
}
