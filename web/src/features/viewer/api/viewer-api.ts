import type {
	CreateViewerSessionInput,
	Heartbeat,
	ListPVCsInput,
	PodSession,
	PVC,
	ViewerAPI,
	ViewerSession,
	ViewerToken,
} from '@/features/viewer/types/viewer'

import { env } from '@/config/env'
import { normalizeViewerError, ViewerApiError } from '@/features/viewer/api/viewer-error'
import Client, { Local } from '@/services/encore/client'

const kubeconfigStorageKey = 'sealos-storage-manager.kubeconfig'

export function readAuthorizationHeader() {
	const configured = import.meta.env.VITE_VIEWER_AUTHORIZATION
	if (configured) {
		return configured
	}
	const stored = globalThis.localStorage?.getItem(kubeconfigStorageKey)
	if (stored) {
		return stored.startsWith('Bearer ') ? stored : `Bearer ${encodeURIComponent(stored)}`
	}
	throw new ViewerApiError({
		code: 'UNAUTHORIZED',
		message: 'Kubeconfig authorization is not configured',
		status: 401,
	})
}

function apiTarget() {
	const base = env.apiBaseUrl
	if (!base || base === '/api') {
		return Local
	}
	return base
}

export function createViewerApi(client = new Client(apiTarget())): ViewerAPI {
	function authorization() {
		return readAuthorizationHeader()
	}

	return {
		async closePodSession(podSessionID: string): Promise<PodSession> {
			try {
				const response = await client.viewer.ClosePodSession(podSessionID, {
					Authorization: authorization(),
				})
				return response.pod_session
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async closeViewerSession(viewerSessionID: string): Promise<ViewerSession> {
			try {
				const response = await client.viewer.CloseViewerSession(viewerSessionID, {
					Authorization: authorization(),
				})
				return response.viewer_session
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async createViewerSession(input: CreateViewerSessionInput): Promise<ViewerSession> {
			try {
				const response = await client.viewer.CreateViewerSession({
					Authorization: authorization(),
					namespace: input.namespace,
					pvc_name: input.pvcName,
				})
				return response.viewer_session
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async getPodSession(podSessionID: string): Promise<PodSession> {
			try {
				const response = await client.viewer.GetPodSession(podSessionID, {
					Authorization: authorization(),
				})
				return response.pod_session
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async getViewerSession(viewerSessionID: string): Promise<ViewerSession> {
			try {
				const response = await client.viewer.GetViewerSession(viewerSessionID, {
					Authorization: authorization(),
				})
				return response.viewer_session
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async heartbeatViewerSession(viewerSessionID: string): Promise<Heartbeat> {
			try {
				const response = await client.viewer.HeartbeatViewerSession(viewerSessionID, {
					Authorization: authorization(),
				})
				return response.heartbeat
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async issueViewerToken(viewerSessionID: string): Promise<ViewerToken> {
			try {
				const response = await client.viewer.IssueViewerToken(viewerSessionID, {
					Authorization: authorization(),
				})
				return response.viewer_token
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async listPVCs(input: ListPVCsInput): Promise<PVC[]> {
			try {
				const response = await client.viewer.ListPVCs({
					Authorization: authorization(),
					Namespace: input.namespace,
				})
				return response.pvc_list.items
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},
	}
}

export const viewerApi = createViewerApi()
