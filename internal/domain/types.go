package domain

import "time"

const (
	AccessModeReadWriteMany    = "ReadWriteMany"
	AccessModeReadOnlyMany     = "ReadOnlyMany"
	AccessModeReadWriteOnce    = "ReadWriteOnce"
	AccessModeReadWriteOncePod = "ReadWriteOncePod"

	ModeReadWrite = "readwrite"
	ModeReadOnly  = "readonly"

	PodStatusCreating    = "creating"
	PodStatusReady       = "ready"
	PodStatusFailed      = "failed"
	PodStatusTerminating = "terminating"
	PodStatusTerminated  = "terminated"

	ViewerStatusActive  = "active"
	ViewerStatusReady   = "ready"
	ViewerStatusClosed  = "closed"
	ViewerStatusExpired = "expired"
	ViewerStatusFailed  = "failed"
)

type PodSession struct {
	ID           string    `json:"id"`
	Namespace    string    `json:"namespace"`
	PVCName      string    `json:"pvc_name"`
	PVCUID       string    `json:"pvc_uid"`
	AccessMode   string    `json:"access_mode"`
	Mode         string    `json:"mode"`
	PodName      string    `json:"pod_name"`
	ServiceName  string    `json:"service_name"`
	ViewerURL    string    `json:"viewer_url"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason"`
	NodeName     string    `json:"node_name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type ViewerSession struct {
	ID              string    `json:"id"`
	PodSessionID    string    `json:"pod_session_id"`
	UserID          string    `json:"-"`
	Username        string    `json:"-"`
	Permission      string    `json:"-"`
	Status          string    `json:"status"`
	PodStatus       string    `json:"pod_status"`
	ViewerURL       string    `json:"viewer_url"`
	Mode            string    `json:"mode"`
	Reason          string    `json:"reason"`
	TokenReady      bool      `json:"token_ready"`
	CreatedAt       time.Time `json:"created_at"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type AuthRequest struct {
	ID              string     `json:"id"`
	ViewerSessionID string     `json:"viewer_session_id"`
	PodSessionID    string     `json:"pod_session_id"`
	Username        string     `json:"username"`
	PasswordHash    string     `json:"-"`
	UsedAt          *time.Time `json:"used_at,omitempty"`
	ExpiresAt       time.Time  `json:"expires_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

type TokenRecord struct {
	TokenHash       string    `json:"-"`
	ViewerSessionID string    `json:"viewer_session_id"`
	PodSessionID    string    `json:"pod_session_id"`
	IssuedAt        time.Time `json:"issued_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type MountedPod struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	NodeName  string `json:"node_name"`
	Phase     string `json:"phase"`
	ReadOnly  bool   `json:"read_only"`
}

type ViewerScheduling struct {
	RequiresNode bool   `json:"requires_node"`
	NodeName     string `json:"node_name"`
	Reason       string `json:"reason"`
}

type PVC struct {
	Namespace        string           `json:"namespace"`
	Name             string           `json:"name"`
	UID              string           `json:"uid"`
	CapacityBytes    int64            `json:"capacity_bytes"`
	Capacity         string           `json:"capacity"`
	AccessModes      []string         `json:"access_modes"`
	Mounted          bool             `json:"mounted"`
	MountedPods      []MountedPod     `json:"mounted_pods"`
	ViewerSupported  bool             `json:"viewer_supported"`
	ViewerMode       string           `json:"viewer_mode"`
	ViewerScheduling ViewerScheduling `json:"viewer_scheduling"`
	Reason           string           `json:"reason"`
}

type PVCMountInfo struct {
	Mounted     bool
	MountedPods []MountedPod
	Nodes       []string
	Conflict    bool
	Reason      string
}

type ViewerToken struct {
	ViewerSessionID string    `json:"viewer_session_id"`
	PodSessionID    string    `json:"pod_session_id"`
	ViewerURL       string    `json:"viewer_url"`
	Token           string    `json:"token"`
	TokenType       string    `json:"token_type"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type Heartbeat struct {
	ViewerSessionID string    `json:"viewer_session_id"`
	Status          string    `json:"status"`
	ServerTime      time.Time `json:"server_time"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type FileBrowserPermissions struct {
	Admin    bool `json:"admin"`
	Execute  bool `json:"execute"`
	Create   bool `json:"create"`
	Rename   bool `json:"rename"`
	Modify   bool `json:"modify"`
	Delete   bool `json:"delete"`
	Share    bool `json:"share"`
	Download bool `json:"download"`
}

type FileBrowserHookVerification struct {
	Allow       bool                   `json:"allow"`
	Reason      string                 `json:"reason"`
	Scope       string                 `json:"scope"`
	Permissions FileBrowserPermissions `json:"permissions"`
}
