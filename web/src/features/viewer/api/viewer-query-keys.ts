export const viewerKeys = {
	all: ['viewer'] as const,
	adminCapabilities: () => [...viewerKeys.all, 'admin', 'capabilities'] as const,
	adminNamespaces: () => [...viewerKeys.all, 'admin', 'namespaces'] as const,
	adminStorageClassDescribe: (name: string) => [...viewerKeys.all, 'admin', 'storage-classes', name, 'describe'] as const,
	adminStorageClassYAML: (name: string) => [...viewerKeys.all, 'admin', 'storage-classes', name, 'yaml'] as const,
	adminStorageClasses: () => [...viewerKeys.all, 'admin', 'storage-classes'] as const,
	context: () => [...viewerKeys.all, 'context'] as const,
	mutations: {
		adminCreateStorageClass: () => [...viewerKeys.all, 'mutation', 'admin-create-storage-class'] as const,
		adminDeleteStorageClass: () => [...viewerKeys.all, 'mutation', 'admin-delete-storage-class'] as const,
		adminUpdateStorageClassPolicy: () => [...viewerKeys.all, 'mutation', 'admin-update-storage-class-policy'] as const,
		adminUpdateStorageClass: () => [...viewerKeys.all, 'mutation', 'admin-update-storage-class'] as const,
		closePodSession: () => [...viewerKeys.all, 'mutation', 'close-pod-session'] as const,
		closeViewerSession: () => [...viewerKeys.all, 'mutation', 'close-viewer-session'] as const,
		createPVC: () => [...viewerKeys.all, 'mutation', 'create-pvc'] as const,
		createViewerSession: () => [...viewerKeys.all, 'mutation', 'create-viewer-session'] as const,
		deletePVC: () => [...viewerKeys.all, 'mutation', 'delete-pvc'] as const,
		expandPVC: () => [...viewerKeys.all, 'mutation', 'expand-pvc'] as const,
		heartbeatViewerSession: () => [...viewerKeys.all, 'mutation', 'heartbeat-viewer-session'] as const,
		issueViewerToken: () => [...viewerKeys.all, 'mutation', 'issue-viewer-token'] as const,
	},
	podSession: (podSessionID: string) =>
		[...viewerKeys.all, 'pod-session', podSessionID] as const,
	pvcs: (namespace: string) => [...viewerKeys.all, 'pvcs', namespace] as const,
	storageClasses: () => [...viewerKeys.all, 'storage-classes'] as const,
	viewerSession: (viewerSessionID: string) =>
		[...viewerKeys.all, 'viewer-session', viewerSessionID] as const,
	viewerToken: (viewerSessionID: string) =>
		[...viewerKeys.all, 'viewer-token', viewerSessionID] as const,
}
