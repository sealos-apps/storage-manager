import type { domain } from '@/services/encore/client'

export type MountedPod = domain.MountedPod
export type PVC = domain.PVC
export type PodSession = domain.PodSession
export type ViewerScheduling = domain.ViewerScheduling
export type ViewerSession = domain.ViewerSession
export type ViewerToken = domain.ViewerToken
export type Heartbeat = domain.Heartbeat

export type ViewerMode = 'readonly' | 'readwrite' | string
export type ViewerStatus = 'active' | 'ready' | 'creating' | 'closed' | 'expired' | 'failed' | string
export type PodStatus = 'creating' | 'ready' | 'failed' | 'terminating' | 'terminated' | string

export interface ViewerApiErrorShape {
	code: ViewerErrorCode | string
	details?: Record<string, unknown>
	message: string
	status?: number
}

export const backendViewerErrorCodes = [
	'PVC_NOT_FOUND',
	'PVC_ACCESS_DENIED',
	'UNSUPPORTED_ACCESS_MODE',
	'PVC_MOUNT_CONFLICT',
	'PVC_MOUNT_PENDING',
	'VIEWER_POD_CREATING',
	'VIEWER_POD_FAILED',
	'VIEWER_SESSION_NOT_FOUND',
	'VIEWER_SESSION_EXPIRED',
	'AUTH_REQUEST_EXPIRED',
	'AUTH_REQUEST_USED',
	'FILEBROWSER_LOGIN_FAILED',
	'HOOK_VERIFY_FAILED',
	'UNAUTHORIZED',
	'VALIDATION_ERROR',
	'INTERNAL_ERROR',
] as const

export type ViewerErrorCode = typeof backendViewerErrorCodes[number]

export interface CreateViewerSessionInput {
	namespace: string
	pvcName: string
}

export interface ListPVCsInput {
	namespace: string
}

export interface ViewerAPI {
	closePodSession: (podSessionID: string) => Promise<PodSession>
	closeViewerSession: (viewerSessionID: string) => Promise<ViewerSession>
	createViewerSession: (input: CreateViewerSessionInput) => Promise<ViewerSession>
	getPodSession: (podSessionID: string) => Promise<PodSession>
	getViewerSession: (viewerSessionID: string) => Promise<ViewerSession>
	heartbeatViewerSession: (viewerSessionID: string) => Promise<Heartbeat>
	issueViewerToken: (viewerSessionID: string) => Promise<ViewerToken>
	listPVCs: (input: ListPVCsInput) => Promise<PVC[]>
}

export interface ViewerSelection {
	namespace: string
	pvcName: string
	uid: string
}
