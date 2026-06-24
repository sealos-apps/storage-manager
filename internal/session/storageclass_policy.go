package session

const (
	ManagedByLabel = "app.kubernetes.io/managed-by"
	ManagedByValue = "sealos-storage-manager"

	StorageClassAvailableToUsersAnnotation = "storage-management.sealos.io/available-to-users"
	StorageClassDisplayNameAnnotation      = "storage-management.sealos.io/display-name"
)
