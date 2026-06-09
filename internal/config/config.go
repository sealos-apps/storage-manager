package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/validation"
)

const DefaultPath = "config/viewer.yaml"
const EnvPath = "CONFIG"

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Admin         AdminConfig         `yaml:"admin"`
	Viewer        ViewerConfig        `yaml:"viewer"`
	Sessions      SessionsConfig      `yaml:"sessions"`
	Cache         CacheConfig         `yaml:"cache"`
	Observability ObservabilityConfig `yaml:"observability"`
	Debug         DebugConfig         `yaml:"debug"`
}

type ServerConfig struct {
	ConfigPath string `yaml:"config_path"`
}

type AdminConfig struct {
	AllowedUserIDs                 []string `yaml:"allowed_user_ids"`
	StorageClassServiceAccountName string   `yaml:"storage_class_service_account_name"`
}

type ViewerConfig struct {
	BackendVerifyURL string               `yaml:"backend_verify_url"`
	HookClientToken  string               `yaml:"hook_client_token"`
	HookScript       string               `yaml:"hook_script"`
	FileManagement   FileManagementConfig `yaml:"file_management"`
	FileBrowser      FileBrowserConfig    `yaml:"filebrowser"`
	Pod              PodConfig            `yaml:"pod"`
	Service          ServiceConfig        `yaml:"service"`
	Ingress          IngressConfig        `yaml:"ingress"`
}

type FileManagementConfig struct {
	Enabled bool `yaml:"enabled"`
}

type FeatureConfig struct {
	FileManagement FileManagementConfig
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
	PublicScheme  string `yaml:"public_scheme"`
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
	ServiceName string       `yaml:"service_name"`
	Logs        LogsConfig   `yaml:"logs"`
	Traces      TracesConfig `yaml:"traces"`
}

type LogsConfig struct {
	Exporter string `yaml:"exporter"`
	Level    string `yaml:"level"`
}

type TracesConfig struct {
	Exporter      string        `yaml:"exporter"`
	Endpoint      string        `yaml:"endpoint"`
	SampleRatio   float64       `yaml:"sample_ratio"`
	BatchTimeout  time.Duration `yaml:"batch_timeout"`
	ExportTimeout time.Duration `yaml:"export_timeout"`
}

type DebugConfig struct {
	Enabled                  bool   `yaml:"enabled"`
	ManagementKubeconfigPath string `yaml:"management_kubeconfig_path"`
	ForcedNamespace          string `yaml:"forced_namespace"`
}

func LoadFile(path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = strings.TrimSpace(os.Getenv(EnvPath))
	}
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
		Admin: AdminConfig{
			StorageClassServiceAccountName: "storageclass-admin",
		},
		Viewer: ViewerConfig{
			FileManagement: FileManagementConfig{
				Enabled: true,
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
			ServiceName: "sealos-storage-manager-viewer",
			Logs: LogsConfig{
				Exporter: "encore",
				Level:    "info",
			},
			Traces: TracesConfig{
				Exporter:      "none",
				SampleRatio:   1,
				BatchTimeout:  5 * time.Second,
				ExportTimeout: 5 * time.Second,
			},
		},
		Debug: DebugConfig{},
	}
}

func (cfg Config) Validate() error {
	var problems []string
	for _, userID := range cfg.Admin.AllowedUserIDs {
		if strings.TrimSpace(userID) == "" {
			problems = append(problems, "admin.allowed_user_ids must not contain empty values")
		}
	}
	if strings.TrimSpace(cfg.Admin.StorageClassServiceAccountName) == "" {
		problems = append(problems, "admin.storage_class_service_account_name is required")
	} else if errs := validation.IsDNS1123Label(strings.TrimSpace(cfg.Admin.StorageClassServiceAccountName)); len(errs) > 0 {
		problems = append(problems, "admin.storage_class_service_account_name must be a DNS-1123 label")
	}
	if strings.TrimSpace(cfg.Viewer.BackendVerifyURL) == "" {
		problems = append(problems, "viewer.backend_verify_url is required")
	}
	if strings.TrimSpace(cfg.Viewer.HookClientToken) == "" {
		problems = append(problems, "viewer.hook_client_token is required")
	}
	if strings.TrimSpace(cfg.Viewer.HookScript) == "" {
		problems = append(problems, "viewer.hook_script is required")
	}
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
	if strings.TrimSpace(cfg.Viewer.Pod.CPURequest) == "" {
		problems = append(problems, "viewer.pod.cpu_request is required")
	}
	if strings.TrimSpace(cfg.Viewer.Pod.MemoryRequest) == "" {
		problems = append(problems, "viewer.pod.memory_request is required")
	}
	if strings.TrimSpace(cfg.Viewer.Pod.CPULimit) == "" {
		problems = append(problems, "viewer.pod.cpu_limit is required")
	}
	if strings.TrimSpace(cfg.Viewer.Pod.MemoryLimit) == "" {
		problems = append(problems, "viewer.pod.memory_limit is required")
	}
	if strings.Contains(cfg.Viewer.Pod.DatabasePath, cfg.Viewer.Pod.MountPath+"/") {
		problems = append(problems, "viewer.pod.database_path must not be inside viewer.pod.mount_path")
	}
	if strings.TrimSpace(cfg.Viewer.Service.Type) == "" {
		problems = append(problems, "viewer.service.type is required")
	}
	if cfg.Viewer.Service.Port <= 0 {
		problems = append(problems, "viewer.service.port must be positive")
	}
	if strings.TrimSpace(cfg.Viewer.Ingress.ClassName) == "" {
		problems = append(problems, "viewer.ingress.class_name is required")
	}
	if strings.TrimSpace(cfg.Viewer.Ingress.HostTemplate) == "" {
		problems = append(problems, "viewer.ingress.host_template is required")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Viewer.Ingress.PublicScheme)) {
	case "", "http", "https":
	default:
		problems = append(problems, "viewer.ingress.public_scheme must be one of http, https")
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
	if strings.TrimSpace(cfg.Observability.ServiceName) == "" {
		problems = append(problems, "observability.service_name is required")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Observability.Logs.Exporter)) {
	case "":
		problems = append(problems, "observability.logs.exporter is required")
	case "encore", "stdout", "discard", "none":
	default:
		problems = append(problems, "observability.logs.exporter must be one of encore, stdout, discard, none")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Observability.Logs.Level)) {
	case "":
		problems = append(problems, "observability.logs.level is required")
	case "debug", "info", "warn", "error":
	default:
		problems = append(problems, "observability.logs.level must be one of debug, info, warn, error")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Observability.Traces.Exporter)) {
	case "":
		problems = append(problems, "observability.traces.exporter is required")
	case "none", "discard", "otlp":
	default:
		problems = append(problems, "observability.traces.exporter must be one of otlp, discard, none")
	}
	if normalizedTraceExporter(cfg.Observability.Traces.Exporter) == "otlp" &&
		strings.TrimSpace(cfg.Observability.Traces.Endpoint) == "" {
		problems = append(problems, "observability.traces.endpoint is required when traces exporter is otlp")
	}
	if cfg.Observability.Traces.SampleRatio < 0 || cfg.Observability.Traces.SampleRatio > 1 {
		problems = append(problems, "observability.traces.sample_ratio must be between 0 and 1")
	}
	if cfg.Observability.Traces.BatchTimeout <= 0 {
		problems = append(problems, "observability.traces.batch_timeout must be positive")
	}
	if cfg.Observability.Traces.ExportTimeout <= 0 {
		problems = append(problems, "observability.traces.export_timeout must be positive")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func (cfg Config) Features() FeatureConfig {
	return FeatureConfig{
		FileManagement: cfg.Viewer.FileManagement,
	}
}

func normalizedTraceExporter(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (cfg Config) Redacted() map[string]any {
	return map[string]any{
		"server": cfg.Server,
		"admin":  cfg.Admin,
		"viewer": map[string]any{
			"backend_verify_url": cfg.Viewer.BackendVerifyURL,
			"hook_client_token":  "redacted",
			"hook_script":        "redacted",
			"file_management":    cfg.Viewer.FileManagement,
			"filebrowser":        cfg.Viewer.FileBrowser,
			"pod":                cfg.Viewer.Pod,
			"service":            cfg.Viewer.Service,
			"ingress":            cfg.Viewer.Ingress,
		},
		"sessions":      cfg.Sessions,
		"cache":         cfg.Cache,
		"observability": cfg.Observability,
		"debug":         cfg.Debug,
	}
}
