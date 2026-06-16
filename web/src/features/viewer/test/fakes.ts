import type {
	AdminNamespace,
	Heartbeat,
	PodSession,
	PVC,
	StorageClass,
	StorageClassDescribe,
	StorageClassYAML,
	StorageQuota,
	ViewerAPI,
	ViewerContext,
	ViewerSession,
	ViewerToken,
} from '@/features/viewer/types/viewer'

export function pvcFixture(overrides: Partial<PVC> = {}): PVC {
	return {
		access_modes: ['ReadWriteOnce'],
		capacity: '10Gi',
		capacity_bytes: 10 * 1024 * 1024 * 1024,
		mount_status: '',
		mounted: false,
		mounted_pods: [],
		name: 'data',
		namespace: 'default',
		reason: '',
		storage_class_name: 'standard',
		uid: 'pvc-uid',
		viewer_mode: 'readwrite',
		viewer_scheduling: {
			node_name: '',
			reason: '',
			requires_node: false,
		},
		viewer_supported: true,
		volume_stats: undefined,
		...overrides,
	}
}

export function viewerContextFixture(overrides: Partial<ViewerContext> = {}): ViewerContext {
	return {
		context_name: 'dev',
		namespace: 'ns-admin',
		...overrides,
	}
}

export function adminNamespaceFixture(overrides: Partial<AdminNamespace> = {}): AdminNamespace {
	return {
		is_current_context: false,
		name: 'kube-system',
		...overrides,
	}
}

export function storageClassFixture(overrides: Partial<StorageClass> = {}): StorageClass {
	return {
		allow_volume_expansion: true,
		creation_timestamp: '2026-05-14T10:00:00Z',
		delete_blocked_reason: '',
		in_use_pvc_count: 0,
		is_default: true,
		managed_by_storage_manager: true,
		name: 'standard',
		provisioner: 'kubernetes.io/no-provisioner',
		reclaim_policy: 'Delete',
		volume_binding_mode: 'Immediate',
		...overrides,
	}
}

export function storageClassYAMLFixture(overrides: Partial<StorageClassYAML> = {}): StorageClassYAML {
	return {
		name: 'standard',
		yaml: [
			'apiVersion: storage.k8s.io/v1',
			'kind: StorageClass',
			'metadata:',
			'  name: standard',
			'provisioner: kubernetes.io/no-provisioner',
		].join('\n'),
		...overrides,
	}
}

export function storageQuotaFixture(overrides: Partial<StorageQuota> = {}): StorageQuota {
	return {
		available_bytes: 15 * 1024 * 1024 * 1024,
		available_quantity: '15Gi',
		limit_bytes: 20 * 1024 * 1024 * 1024,
		limit_quantity: '20Gi',
		used_bytes: 5 * 1024 * 1024 * 1024,
		used_quantity: '5Gi',
		...overrides,
	}
}

export function storageClassDescribeFixture(overrides: Partial<StorageClassDescribe> = {}): StorageClassDescribe {
	return {
		describe: 'Name: standard\nProvisioner: kubernetes.io/no-provisioner',
		name: 'standard',
		...overrides,
	}
}

export function viewerSessionFixture(overrides: Partial<ViewerSession> = {}): ViewerSession {
	return {
		created_at: '2026-05-14T10:00:00Z',
		expires_at: '2026-05-14T10:03:00Z',
		id: 'vs_1',
		last_heartbeat_at: '2026-05-14T10:00:00Z',
		mode: 'readwrite',
		namespace: 'default',
		pod_session_id: 'ps_1',
		pod_status: 'creating',
		pvc_name: 'data',
		reason: '',
		status: 'creating',
		token_ready: false,
		viewer_url: 'https://viewer.example.test',
		...overrides,
	}
}

export function viewerTokenFixture(overrides: Partial<ViewerToken> = {}): ViewerToken {
	return {
		expires_at: '2026-05-14T10:30:00Z',
		pod_session_id: 'ps_1',
		token: 'fb-token',
		token_type: 'Bearer',
		viewer_session_id: 'vs_1',
		viewer_url: 'https://viewer.example.test',
		...overrides,
	}
}

export function podSessionFixture(overrides: Partial<PodSession> = {}): PodSession {
	return {
		access_mode: 'ReadWriteOnce',
		created_at: '2026-05-14T10:00:00Z',
		expires_at: '2026-05-14T10:12:00Z',
		id: 'ps_1',
		last_active_at: '2026-05-14T10:00:00Z',
		mode: 'readwrite',
		namespace: 'default',
		node_name: '',
		pod_name: 'viewer-ps-1',
		pvc_name: 'data',
		pvc_uid: 'pvc-uid',
		reason: '',
		runtime_version: 'default',
		service_name: 'viewer-ps-1',
		status: 'ready',
		updated_at: '2026-05-14T10:00:05Z',
		viewer_url: 'https://viewer.example.test',
		...overrides,
	}
}

export function heartbeatFixture(overrides: Partial<Heartbeat> = {}): Heartbeat {
	return {
		expires_at: '2026-05-14T10:03:00Z',
		server_time: '2026-05-14T10:00:20Z',
		status: 'active',
		viewer_session_id: 'vs_1',
		...overrides,
	}
}

export function createFakeViewerAPI(overrides: Partial<ViewerAPI> = {}): ViewerAPI {
	return {
		adminCapabilities: async () => ({
			can_manage_pvcs: false,
			can_manage_storage_classes: false,
			file_management_enabled: true,
			pvc_creation_enabled: true,
			user_namespace: 'ns-admin',
		}),
		adminCreateStorageClass: async () => storageClassFixture(),
		adminDeleteStorageClass: async name => storageClassFixture({ name }),
		adminDescribeStorageClass: async name => storageClassDescribeFixture({ name }),
		adminGetStorageClassYAML: async name => storageClassYAMLFixture({ name }),
		adminListNamespaces: async () => [
			adminNamespaceFixture({ name: 'ns-admin', is_current_context: true }),
			adminNamespaceFixture({ name: 'kube-system' }),
		],
		adminListStorageClasses: async () => [storageClassFixture()],
		adminUpdateStorageClass: async name => storageClassFixture({ name }),
		closePodSession: async () => podSessionFixture({ status: 'terminated' }),
		closeViewerSession: async id => viewerSessionFixture({ id, status: 'closed' }),
		createPVC: async input =>
			pvcFixture({
				namespace: input.namespace,
				name: input.name,
				capacity: input.capacity,
				capacity_bytes: input.capacityBytes,
				access_modes: input.accessModes,
			}),
		createViewerSession: async input =>
			viewerSessionFixture({
				id: 'vs_created',
				pod_session_id: 'ps_created',
				status: 'creating',
				viewer_url: '',
				...(
					input.pvcName
						? {}
						: { reason: 'missing pvc' }
				),
			}),
		deletePVC: async input =>
			pvcFixture({
				namespace: input.namespace,
				name: input.name,
			}),
		expandPVC: async input =>
			pvcFixture({
				namespace: input.namespace,
				name: input.name,
				capacity: input.capacity,
				capacity_bytes: input.capacityBytes,
			}),
		getContext: async () => viewerContextFixture(),
		getStorageQuota: async () => storageQuotaFixture(),
		getPodSession: async id => podSessionFixture({ id }),
		getViewerSession: async id => viewerSessionFixture({ id, status: 'ready', token_ready: true }),
		heartbeatViewerSession: async id => heartbeatFixture({ viewer_session_id: id }),
		issueViewerToken: async id => viewerTokenFixture({ viewer_session_id: id }),
		listPVCs: async () => [pvcFixture()],
		listStorageClasses: async () => [storageClassFixture()],
		...overrides,
	}
}
