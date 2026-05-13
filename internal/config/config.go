package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const DefaultPath = "config/viewer.yaml"

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Kubernetes    KubernetesConfig    `yaml:"kubernetes"`
	Viewer        ViewerConfig        `yaml:"viewer"`
	Sessions      SessionsConfig      `yaml:"sessions"`
	Cache         CacheConfig         `yaml:"cache"`
	Observability ObservabilityConfig `yaml:"observability"`
	Integration   IntegrationConfig   `yaml:"integration"`
}

type ServerConfig struct {
	ConfigPath string `yaml:"config_path"`
}

type KubernetesConfig struct {
	ManagementKubeconfigPath string `yaml:"management_kubeconfig_path"`
}

type ViewerConfig struct {
	NamespaceAllowlist []string          `yaml:"namespace_allowlist"`
	BackendVerifyURL   string            `yaml:"backend_verify_url"`
	HookClientToken    string            `yaml:"hook_client_token"`
	FileBrowser        FileBrowserConfig `yaml:"filebrowser"`
	Pod                PodConfig         `yaml:"pod"`
	Service            ServiceConfig     `yaml:"service"`
	Ingress            IngressConfig     `yaml:"ingress"`
}

type FileBrowserConfig struct {
	Image        string        `yaml:"image"`
	BinaryPath   string        `yaml:"binary_path"`
	Port         int32         `yaml:"port"`
	TokenTTL     time.Duration `yaml:"token_ttl"`
	LoginTimeout time.Duration `yaml:"login_timeout"`
}

type PodConfig struct {
	ServiceAccountName string `yaml:"service_account_name"`
	MountPath          string `yaml:"mount_path"`
	DatabasePath       string `yaml:"database_path"`
	CPURequest         string `yaml:"cpu_request"`
	MemoryRequest      string `yaml:"memory_request"`
	CPULimit           string `yaml:"cpu_limit"`
	MemoryLimit        string `yaml:"memory_limit"`
}

type ServiceConfig struct {
	Type string `yaml:"type"`
	Port int32  `yaml:"port"`
}

type IngressConfig struct {
	ClassName     string `yaml:"class_name"`
	HostTemplate  string `yaml:"host_template"`
	TLSSecretName string `yaml:"tls_secret_name"`
}

type SessionsConfig struct {
	HeartbeatInterval   time.Duration `yaml:"heartbeat_interval"`
	ViewerSessionTimout time.Duration `yaml:"viewer_session_timeout"`
	PodKeepaliveGrace   time.Duration `yaml:"pod_keepalive_grace"`
	AuthRequestTTL      time.Duration `yaml:"auth_request_ttl"`
	RecoveryGrace       time.Duration `yaml:"recovery_grace"`
	OrphanGrace         time.Duration `yaml:"orphan_grace"`
}

type CacheConfig struct {
	PodSessionsMaxEntries    int           `yaml:"pod_sessions_max_entries"`
	ViewerSessionsMaxEntries int           `yaml:"viewer_sessions_max_entries"`
	AuthRequestsMaxEntries   int           `yaml:"auth_requests_max_entries"`
	TokenRecordsMaxEntries   int           `yaml:"token_records_max_entries"`
	IndexesMaxEntries        int           `yaml:"indexes_max_entries"`
	PurgeInterval            time.Duration `yaml:"purge_interval"`
	ReconcileInterval        time.Duration `yaml:"reconcile_interval"`
}

type ObservabilityConfig struct {
	ServiceName      string  `yaml:"service_name"`
	MetricsEnabled   bool    `yaml:"metrics_enabled"`
	MetricsPath      string  `yaml:"metrics_path"`
	TracingEnabled   bool    `yaml:"tracing_enabled"`
	TraceSampleRatio float64 `yaml:"trace_sample_ratio"`
	OTLPEndpoint     string  `yaml:"otlp_endpoint"`
	LogLevel         string  `yaml:"log_level"`
}

type IntegrationConfig struct {
	KubeconfigPath           string `yaml:"kubeconfig_path"`
	ManagementKubeconfigPath string `yaml:"management_kubeconfig_path"`
	Namespace                string `yaml:"namespace"`
	StorageClassName         string `yaml:"storage_class_name"`
}

func LoadFile(path string) (Config, error) {
	if path == "" {
		path = DefaultPath
	}

	data, err := os.ReadFile(path) //nolint:gosec // Config path is an explicit local/deployment input, not user-supplied request data.
	if err != nil {
		return Config{}, fmt.Errorf("reading config file: %w", err)
	}

	cfg, err := Load(data)
	if err != nil {
		return Config{}, err
	}
	cfg.Server.ConfigPath = path
	return cfg, nil
}

func Load(data []byte) (Config, error) {
	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config yaml: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			ConfigPath: DefaultPath,
		},
		Kubernetes: KubernetesConfig{},
		Viewer: ViewerConfig{
			NamespaceAllowlist: []string{},
			BackendVerifyURL:   "http://viewer-backend/internal/filebrowser-hook/verify",
			HookClientToken:    "",
			FileBrowser: FileBrowserConfig{
				Image:        "filebrowser/filebrowser:v2.30.0",
				BinaryPath:   "/filebrowser",
				Port:         8080,
				TokenTTL:     15 * time.Minute,
				LoginTimeout: 2 * time.Second,
			},
			Pod: PodConfig{
				MountPath:     "/srv",
				DatabasePath:  "/tmp/filebrowser.db",
				CPURequest:    "50m",
				MemoryRequest: "64Mi",
				CPULimit:      "500m",
				MemoryLimit:   "512Mi",
			},
			Service: ServiceConfig{
				Type: "ClusterIP",
				Port: 80,
			},
			Ingress: IngressConfig{
				ClassName:    "nginx",
				HostTemplate: "viewer-{{ .PodSessionID }}.example.com",
			},
		},
		Sessions: SessionsConfig{
			HeartbeatInterval:   30 * time.Second,
			ViewerSessionTimout: 90 * time.Second,
			PodKeepaliveGrace:   5 * time.Minute,
			AuthRequestTTL:      30 * time.Second,
			RecoveryGrace:       3 * time.Minute,
			OrphanGrace:         10 * time.Minute,
		},
		Cache: CacheConfig{
			PodSessionsMaxEntries:    1024,
			ViewerSessionsMaxEntries: 4096,
			AuthRequestsMaxEntries:   4096,
			TokenRecordsMaxEntries:   4096,
			IndexesMaxEntries:        4096,
			PurgeInterval:            30 * time.Second,
			ReconcileInterval:        45 * time.Second,
		},
		Observability: ObservabilityConfig{
			ServiceName:      "sealos-storage-manager-viewer",
			MetricsEnabled:   true,
			MetricsPath:      "/metrics",
			TracingEnabled:   true,
			TraceSampleRatio: 1,
			LogLevel:         "info",
		},
		Integration: IntegrationConfig{
			KubeconfigPath:           "kubeconfig.test.yaml",
			ManagementKubeconfigPath: "kubeconfig.management.yaml",
			Namespace:                "default",
		},
	}
}

func (cfg Config) Validate() error {
	var problems []string
	if strings.TrimSpace(cfg.Viewer.FileBrowser.Image) == "" {
		problems = append(problems, "viewer.filebrowser.image is required")
	}
	if strings.TrimSpace(cfg.Viewer.FileBrowser.BinaryPath) == "" {
		problems = append(problems, "viewer.filebrowser.binary_path is required")
	}
	if cfg.Viewer.FileBrowser.Port <= 0 {
		problems = append(problems, "viewer.filebrowser.port must be positive")
	}
	if cfg.Viewer.FileBrowser.TokenTTL <= 0 {
		problems = append(problems, "viewer.filebrowser.token_ttl must be positive")
	}
	if cfg.Viewer.FileBrowser.LoginTimeout <= 0 {
		problems = append(problems, "viewer.filebrowser.login_timeout must be positive")
	}
	if strings.TrimSpace(cfg.Viewer.Pod.MountPath) == "" {
		problems = append(problems, "viewer.pod.mount_path is required")
	}
	if strings.TrimSpace(cfg.Viewer.Pod.DatabasePath) == "" {
		problems = append(problems, "viewer.pod.database_path is required")
	}
	if strings.Contains(cfg.Viewer.Pod.DatabasePath, cfg.Viewer.Pod.MountPath+"/") {
		problems = append(problems, "viewer.pod.database_path must not be inside viewer.pod.mount_path")
	}
	if strings.TrimSpace(cfg.Viewer.Ingress.HostTemplate) == "" {
		problems = append(problems, "viewer.ingress.host_template is required")
	}
	if cfg.Sessions.ViewerSessionTimout <= 0 {
		problems = append(problems, "sessions.viewer_session_timeout must be positive")
	}
	if cfg.Sessions.AuthRequestTTL <= 0 {
		problems = append(problems, "sessions.auth_request_ttl must be positive")
	}
	if cfg.Cache.PodSessionsMaxEntries <= 0 ||
		cfg.Cache.ViewerSessionsMaxEntries <= 0 ||
		cfg.Cache.AuthRequestsMaxEntries <= 0 ||
		cfg.Cache.TokenRecordsMaxEntries <= 0 ||
		cfg.Cache.IndexesMaxEntries <= 0 {
		problems = append(problems, "cache max entry values must be positive")
	}
	if cfg.Observability.TraceSampleRatio < 0 || cfg.Observability.TraceSampleRatio > 1 {
		problems = append(problems, "observability.trace_sample_ratio must be between 0 and 1")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func (cfg Config) Redacted() map[string]any {
	return map[string]any{
		"server":     cfg.Server,
		"kubernetes": cfg.Kubernetes,
		"viewer": map[string]any{
			"namespace_allowlist": cfg.Viewer.NamespaceAllowlist,
			"backend_verify_url":  cfg.Viewer.BackendVerifyURL,
			"hook_client_token":   "redacted",
			"filebrowser":         cfg.Viewer.FileBrowser,
			"pod":                 cfg.Viewer.Pod,
			"service":             cfg.Viewer.Service,
			"ingress":             cfg.Viewer.Ingress,
		},
		"sessions":      cfg.Sessions,
		"cache":         cfg.Cache,
		"observability": cfg.Observability,
		"integration": map[string]any{
			"kubeconfig_path":    cfg.Integration.KubeconfigPath,
			"namespace":          cfg.Integration.Namespace,
			"storage_class_name": cfg.Integration.StorageClassName,
		},
	}
}
