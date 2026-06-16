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
	// ID uniquely identifies the backing viewer pod session.
	ID string `json:"id"`
	// Namespace is the Kubernetes namespace containing the PVC and viewer resources.
	Namespace string `json:"namespace"`
	// PVCName is the PersistentVolumeClaim mounted into the viewer pod.
	PVCName string `json:"pvc_name"`
	// PVCUID is the Kubernetes UID of the mounted PersistentVolumeClaim.
	PVCUID string `json:"pvc_uid"`
	// AccessMode is the PVC access mode selected for the viewer pod.
	AccessMode string `json:"access_mode"`
	// Mode is the effective viewer permission mode, such as readonly or readwrite.
	Mode string `json:"mode"`
	// PodName is the Kubernetes pod running File Browser.
	PodName string `json:"pod_name"`
	// ServiceName is the Kubernetes Service exposing the viewer pod.
	ServiceName string `json:"service_name"`
	// ViewerURL is the public URL clients use to open File Browser.
	ViewerURL         string `json:"viewer_url"`
	InternalViewerURL string `json:"-"`
	// RuntimeVersion is the File Browser runtime image or application version.
	RuntimeVersion string `json:"runtime_version"`
	// Status is the pod session lifecycle state.
	Status string `json:"status"`
	// Reason explains why the pod session is pending, failed, or otherwise limited.
	Reason string `json:"reason"`
	// NodeName is the Kubernetes node selected for the viewer pod when scheduling requires one.
	NodeName string `json:"node_name"`
	// CreatedAt is the time the pod session record was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the last time the pod session record changed.
	UpdatedAt time.Time `json:"updated_at"`
	// LastActiveAt is the last observed activity time for this pod session.
	LastActiveAt time.Time `json:"last_active_at"`
	// ExpiresAt is when the pod session becomes eligible for cleanup.
	ExpiresAt    time.Time `json:"expires_at"`
	AdminContext bool      `json:"-"`
}

type ViewerSession struct {
	// ID uniquely identifies the user-facing viewer session.
	ID string `json:"id"`
	// PodSessionID identifies the backing pod session used by this viewer session.
	PodSessionID string `json:"pod_session_id"`
	// Namespace is the Kubernetes namespace containing the PVC.
	Namespace string `json:"namespace"`
	// PVCName is the PersistentVolumeClaim exposed in File Browser.
	PVCName    string `json:"pvc_name"`
	UserID     string `json:"-"`
	Username   string `json:"-"`
	Permission string `json:"-"`
	// Status is the viewer session lifecycle state.
	Status string `json:"status"`
	// PodStatus is the lifecycle state of the backing pod session.
	PodStatus string `json:"pod_status"`
	// ViewerURL is the public URL clients use to open File Browser.
	ViewerURL string `json:"viewer_url"`
	// Mode is the effective File Browser permission mode for this viewer.
	Mode string `json:"mode"`
	// Reason explains why the viewer is pending, failed, expired, or limited.
	Reason string `json:"reason"`
	// TokenReady reports whether a File Browser login token can be issued.
	TokenReady bool `json:"token_ready"`
	// CreatedAt is the time the viewer session was created.
	CreatedAt time.Time `json:"created_at"`
	// LastHeartbeatAt is the last heartbeat received from the client.
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	// ExpiresAt is when the viewer session expires without another heartbeat.
	ExpiresAt    time.Time `json:"expires_at"`
	AdminContext bool      `json:"-"`
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
	// Namespace is the Kubernetes namespace containing the pod.
	Namespace string `json:"namespace"`
	// Name is the Kubernetes pod name.
	Name string `json:"name"`
	// NodeName is the node currently running the pod.
	NodeName string `json:"node_name"`
	// Phase is the Kubernetes pod phase.
	Phase string `json:"phase"`
	// ReadOnly reports whether the PVC is mounted read-only by this pod.
	ReadOnly bool `json:"read_only"`
}

type ViewerScheduling struct {
	// RequiresNode reports whether the viewer pod must be scheduled onto a specific node.
	RequiresNode bool `json:"requires_node"`
	// NodeName is the required node when node affinity is needed.
	NodeName string `json:"node_name"`
	// Reason explains why node-specific scheduling is required or unavailable.
	Reason string `json:"reason"`
}

type PVCVolumeStats struct {
	// Source identifies where the usage sample came from, such as kubelet metrics.
	Source string `json:"source"`
	// Status describes whether the usage sample is current, unavailable, or partial.
	Status string `json:"status"`
	// SampleTime is when the usage metrics were collected.
	SampleTime *time.Time `json:"sample_time,omitempty"`
	// UsedBytes is the measured bytes currently used by the PVC.
	UsedBytes int64 `json:"used_bytes"`
	// MetricCapacityBytes is the capacity reported by metrics for this PVC.
	MetricCapacityBytes int64 `json:"metric_capacity_bytes"`
	// AvailableBytes is the measured bytes still available on the PVC filesystem.
	AvailableBytes int64 `json:"available_bytes"`
}

type PVC struct {
	// Namespace is the Kubernetes namespace containing the PVC.
	Namespace string `json:"namespace"`
	// Name is the Kubernetes PersistentVolumeClaim name.
	Name string `json:"name"`
	// UID is the Kubernetes UID of the PVC.
	UID string `json:"uid"`
	// CapacityBytes is the requested PVC capacity in bytes.
	CapacityBytes int64 `json:"capacity_bytes"`
	// Capacity is the requested PVC capacity as a Kubernetes quantity string.
	Capacity string `json:"capacity"`
	// AccessModes are the Kubernetes access modes requested by the PVC.
	AccessModes []string `json:"access_modes"`
	// StorageClassName is the StorageClass used by the PVC.
	StorageClassName string `json:"storage_class_name"`
	// Mounted reports whether active pods currently mount this PVC.
	Mounted bool `json:"mounted"`
	// MountedPods lists active pods that currently mount this PVC.
	MountedPods []MountedPod `json:"mounted_pods"`
	// ViewerSupported reports whether storage-manager can open a File Browser viewer for this PVC.
	ViewerSupported bool `json:"viewer_supported"`
	// ViewerMode is the effective viewer permission mode for this PVC.
	ViewerMode string `json:"viewer_mode"`
	// ViewerScheduling describes node scheduling constraints for a viewer pod.
	ViewerScheduling ViewerScheduling `json:"viewer_scheduling"`
	// Reason explains why viewer access is limited or unavailable.
	Reason string `json:"reason"`
	// VolumeStats contains optional PVC filesystem usage metrics.
	VolumeStats *PVCVolumeStats `json:"volume_stats,omitempty"`
}

type Namespace struct {
	// Name is the Kubernetes namespace name.
	Name string `json:"name"`
	// IsCurrentContext reports whether this namespace matches the caller's current context.
	IsCurrentContext bool `json:"is_current_context"`
}

type StorageClass struct {
	// Name is the Kubernetes StorageClass name.
	Name string `json:"name"`
	// Provisioner is the CSI or in-tree provisioner used by the StorageClass.
	Provisioner string `json:"provisioner"`
	// AllowVolumeExpansion reports whether PVCs using this class can be expanded.
	AllowVolumeExpansion bool `json:"allow_volume_expansion"`
	// VolumeBindingMode is the Kubernetes binding mode for volumes using this class.
	VolumeBindingMode string `json:"volume_binding_mode"`
	// IsDefault reports whether Kubernetes marks this as the default StorageClass.
	IsDefault bool `json:"is_default"`
	// ReclaimPolicy is the reclaim policy applied to dynamically provisioned volumes.
	ReclaimPolicy string `json:"reclaim_policy"`
	// CreationTimestampRFC3339 is the StorageClass creation time formatted as RFC3339.
	CreationTimestampRFC3339 string `json:"creation_timestamp"`
	// ManagedByStorageManager reports whether this class was created through storage-manager.
	ManagedByStorageManager bool `json:"managed_by_storage_manager"`
	// DeleteBlockedReason explains why this StorageClass cannot currently be deleted.
	DeleteBlockedReason string `json:"delete_blocked_reason"`
	// InUsePVCCount is the number of PVCs currently referencing this StorageClass.
	InUsePVCCount int `json:"in_use_pvc_count"`
}

type PVCMountInfo struct {
	Mounted     bool
	MountedPods []MountedPod
	Nodes       []string
	Conflict    bool
	Reason      string
}

type ViewerToken struct {
	// ViewerSessionID identifies the viewer session that owns this token.
	ViewerSessionID string `json:"viewer_session_id"`
	// PodSessionID identifies the backing pod session.
	PodSessionID string `json:"pod_session_id"`
	// ViewerURL is the File Browser URL associated with the token.
	ViewerURL string `json:"viewer_url"`
	// Token is the short-lived bearer token accepted by File Browser.
	Token string `json:"token"`
	// TokenType describes how the token should be presented to File Browser.
	TokenType string `json:"token_type"`
	// ExpiresAt is when the File Browser token expires.
	ExpiresAt time.Time `json:"expires_at"`
}

type Heartbeat struct {
	// ViewerSessionID identifies the refreshed viewer session.
	ViewerSessionID string `json:"viewer_session_id"`
	// Status is the viewer session status after the heartbeat.
	Status string `json:"status"`
	// ServerTime is the backend time when the heartbeat was accepted.
	ServerTime time.Time `json:"server_time"`
	// ExpiresAt is the new viewer session expiration time.
	ExpiresAt time.Time `json:"expires_at"`
}

type FileBrowserPermissions struct {
	// Admin grants File Browser administrator privileges.
	Admin bool `json:"admin"`
	// Execute allows command execution in File Browser.
	Execute bool `json:"execute"`
	// Create allows creating files and directories.
	Create bool `json:"create"`
	// Rename allows renaming files and directories.
	Rename bool `json:"rename"`
	// Modify allows editing file contents.
	Modify bool `json:"modify"`
	// Delete allows deleting files and directories.
	Delete bool `json:"delete"`
	// Share allows creating File Browser shares.
	Share bool `json:"share"`
	// Download allows downloading files.
	Download bool `json:"download"`
}

type FileBrowserHookVerification struct {
	// Allow reports whether File Browser should allow the login attempt.
	Allow bool `json:"allow"`
	// Reason explains the decision for audit and troubleshooting.
	Reason string `json:"reason"`
	// Scope describes the filesystem scope granted to File Browser.
	Scope string `json:"scope"`
	// Permissions are the File Browser capabilities granted to the session.
	Permissions FileBrowserPermissions `json:"permissions"`
}
