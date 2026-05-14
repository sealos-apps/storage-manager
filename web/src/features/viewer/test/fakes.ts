import type {
	Heartbeat,
	PodSession,
	PVC,
	ViewerAPI,
	ViewerSession,
	ViewerToken,
} from '@/features/viewer/types/viewer'

export function pvcFixture(overrides: Partial<PVC> = {}): PVC {
	return {
		access_modes: ['ReadWriteOnce'],
		capacity: '10Gi',
		capacity_bytes: 10 * 1024 * 1024 * 1024,
		mounted: false,
		mounted_pods: [],
		name: 'data',
		namespace: 'default',
		reason: '',
		uid: 'pvc-uid',
		viewer_mode: 'readwrite',
		viewer_scheduling: {
			node_name: '',
			reason: '',
			requires_node: false,
		},
		viewer_supported: true,
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
		pod_session_id: 'ps_1',
		pod_status: 'creating',
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
		closePodSession: async () => podSessionFixture({ status: 'terminated' }),
		closeViewerSession: async id => viewerSessionFixture({ id, status: 'closed' }),
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
		getPodSession: async id => podSessionFixture({ id }),
		getViewerSession: async id => viewerSessionFixture({ id, status: 'ready', token_ready: true }),
		heartbeatViewerSession: async id => heartbeatFixture({ viewer_session_id: id }),
		issueViewerToken: async id => viewerTokenFixture({ viewer_session_id: id }),
		listPVCs: async () => [pvcFixture()],
		...overrides,
	}
}
