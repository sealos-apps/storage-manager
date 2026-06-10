import type { domain, session, viewer } from '@sealos-storage-manager/encore-client'

export type MountedPod = domain.MountedPod
export type PVC = domain.PVC
export type PodSession = domain.PodSession
export type ViewerScheduling = domain.ViewerScheduling
export type ViewerSession = domain.ViewerSession
export type ViewerToken = domain.ViewerToken
export type Heartbeat = domain.Heartbeat
export type StorageClass = domain.StorageClass
export type StorageClassDescribe = session.StorageClassDescribe
export type StorageClassYAML = session.StorageClassYAML
export type AdminNamespace = domain.Namespace
export type ViewerContext = viewer.ViewerContext

export type ViewerMode = 'readonly' | 'readwrite' | string
export type ViewerStatus = 'active' | 'ready' | 'creating' | 'closed' | 'expired' | 'failed' | string
export type PodStatus = 'creating' | 'ready' | 'failed' | 'terminating' | 'terminated' | string

export interface ViewerApiErrorShape {
	code: ViewerErrorCode
	details?: Record<string, unknown>
	message: string
	status?: number
}

export const backendViewerErrorCodes = [
	'PVC_NOT_FOUND',
	'PVC_ALREADY_EXISTS',
	'PVC_IN_USE',
	'PVC_ACCESS_DENIED',
	'PVC_CREATE_FORBIDDEN',
	'PVC_DELETE_FORBIDDEN',
	'PVC_EXPAND_FORBIDDEN',
	'PVC_EXPAND_UNSUPPORTED',
	'PVC_EXPAND_NOT_INCREASED',
	'UNSUPPORTED_ACCESS_MODE',
	'PVC_MOUNT_CONFLICT',
	'PVC_MOUNT_PENDING',
	'STORAGE_CLASS_NOT_FOUND',
	'STORAGE_CLASS_NOT_VISIBLE',
	'STORAGE_CLASS_YAML_INVALID',
	'STORAGE_CLASS_CONFLICT',
	'STORAGE_CLASS_DELETE_FORBIDDEN',
	'STORAGE_CLASS_IN_USE',
	'ADMIN_ACCESS_DENIED',
	'VIEWER_POD_CREATING',
	'VIEWER_POD_FAILED',
	'POD_SESSION_NOT_FOUND',
	'VIEWER_SESSION_NOT_FOUND',
	'VIEWER_SESSION_EXPIRED',
	'AUTH_REQUEST_EXPIRED',
	'AUTH_REQUEST_USED',
	'FILEBROWSER_LOGIN_FAILED',
	'FILE_MANAGEMENT_DISABLED',
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

export interface CreatePVCInput {
	accessModes: string[]
	capacity: string
	capacityBytes: number
	name: string
	namespace: string
	storageClassName: string
}

export interface AdminCapabilities {
	can_manage_pvcs: boolean
	can_manage_storage_classes: boolean
	file_management_enabled: boolean
	user_namespace: string
}

export interface StorageClassYAMLInput {
	yaml: string
}

export interface StorageClassPolicyInput {
	allowedAccessModes: string[]
	visibleInCreate: boolean
}

export interface DeletePVCInput {
	name: string
	namespace: string
}

export interface ExpandPVCInput {
	capacity: string
	capacityBytes: number
	name: string
	namespace: string
}

export interface ListPVCsInput {
	namespace: string
}

export interface ViewerAPI {
	adminCapabilities: () => Promise<AdminCapabilities>
	adminCreateStorageClass: (input: StorageClassYAMLInput) => Promise<StorageClass>
	adminDeleteStorageClass: (name: string) => Promise<StorageClass>
	adminDescribeStorageClass: (name: string) => Promise<StorageClassDescribe>
	adminGetStorageClassYAML: (name: string) => Promise<StorageClassYAML>
	adminListNamespaces: () => Promise<AdminNamespace[]>
	adminListStorageClasses: () => Promise<StorageClass[]>
	adminUpdateStorageClassPolicy: (name: string, input: StorageClassPolicyInput) => Promise<StorageClass>
	adminUpdateStorageClass: (name: string, input: StorageClassYAMLInput) => Promise<StorageClass>
	closePodSession: (podSessionID: string) => Promise<PodSession>
	closeViewerSession: (viewerSessionID: string) => Promise<ViewerSession>
	createPVC: (input: CreatePVCInput) => Promise<PVC>
	createViewerSession: (input: CreateViewerSessionInput) => Promise<ViewerSession>
	deletePVC: (input: DeletePVCInput) => Promise<PVC>
	expandPVC: (input: ExpandPVCInput) => Promise<PVC>
	getPodSession: (podSessionID: string) => Promise<PodSession>
	getContext: () => Promise<ViewerContext>
	getViewerSession: (viewerSessionID: string) => Promise<ViewerSession>
	heartbeatViewerSession: (viewerSessionID: string) => Promise<Heartbeat>
	issueViewerToken: (viewerSessionID: string) => Promise<ViewerToken>
	listPVCs: (input: ListPVCsInput) => Promise<PVC[]>
	listStorageClasses: () => Promise<StorageClass[]>
}

export interface ViewerSelection {
	namespace: string
	pvcName: string
	uid: string
}
