import type {
	AdminCapabilities,
	AdminNamespace,
	CreatePVCInput,
	CreateViewerSessionInput,
	DeletePVCInput,
	ExpandPVCInput,
	GetStorageQuotaInput,
	Heartbeat,
	ListPVCsInput,
	PodSession,
	PVC,
	StorageClass,
	StorageClassDescribe,
	StorageClassYAML,
	StorageClassYAMLInput,
	StorageQuota,
	ViewerAPI,
	ViewerContext,
	ViewerSession,
	ViewerToken,
} from '@/features/viewer/types/viewer'

import Client from '@sealos-storage-manager/encore-client'

import { env } from '@/config/env'
import { normalizeViewerError } from '@/features/viewer/api/viewer-error'
import { getCachedAccountAuthorizationHeader, getCachedAuthorizationHeader } from '@/services/sealos/sealos-authorization'

export function readAuthorizationHeader() {
	return getCachedAuthorizationHeader()
}

export function apiTarget() {
	const base = env.apiBaseUrl
	if (!base) {
		return ''
	}
	return base.replace(/\/+$/, '')
}

type ViewerClient = Pick<Client, 'viewer'>

export function createViewerApi(
	client?: ViewerClient,
	fetcher?: typeof fetch,
): ViewerAPI {
	const activeClient = client ?? new Client(apiTarget(), fetcher ? { fetcher } : undefined)
	function authorization() {
		return readAuthorizationHeader()
	}
	function accountAuthorization() {
		return getCachedAccountAuthorizationHeader()
	}

	return {
		async adminCapabilities(): Promise<AdminCapabilities> {
			try {
				const response = await activeClient.viewer.AdminCapabilities({
					Authorization: authorization(),
				})
				return response.admin_capabilities
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async adminCreateStorageClass(input: StorageClassYAMLInput): Promise<StorageClass> {
			try {
				const response = await activeClient.viewer.AdminCreateStorageClass({
					Authorization: authorization(),
					yaml: input.yaml,
				})
				return response.storage_class
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async adminDeleteStorageClass(name: string): Promise<StorageClass> {
			try {
				const response = await activeClient.viewer.AdminDeleteStorageClass(name, {
					Authorization: authorization(),
				})
				return response.storage_class
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async adminDescribeStorageClass(name: string): Promise<StorageClassDescribe> {
			try {
				const response = await activeClient.viewer.AdminDescribeStorageClass(name, {
					Authorization: authorization(),
				})
				return response.storage_class_describe
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async adminGetStorageClassYAML(name: string): Promise<StorageClassYAML> {
			try {
				const response = await activeClient.viewer.AdminGetStorageClassYAML(name, {
					Authorization: authorization(),
				})
				return response.storage_class_yaml
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async adminListNamespaces(): Promise<AdminNamespace[]> {
			try {
				const response = await activeClient.viewer.AdminListNamespaces({
					Authorization: authorization(),
				})
				return response.namespace_list.items
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async adminListStorageClasses(): Promise<StorageClass[]> {
			try {
				const response = await activeClient.viewer.AdminListStorageClasses({
					Authorization: authorization(),
				})
				return response.storage_class_list.items
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async adminUpdateStorageClass(name: string, input: StorageClassYAMLInput): Promise<StorageClass> {
			try {
				const response = await activeClient.viewer.AdminUpdateStorageClass(name, {
					Authorization: authorization(),
					yaml: input.yaml,
				})
				return response.storage_class
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async closePodSession(podSessionID: string): Promise<PodSession> {
			try {
				const response = await activeClient.viewer.ClosePodSession(podSessionID, {
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
				const response = await activeClient.viewer.CloseViewerSession(viewerSessionID, {
					Authorization: authorization(),
				})
				return response.viewer_session
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async createPVC(input: CreatePVCInput): Promise<PVC> {
			try {
				const response = await activeClient.viewer.CreatePVC({
					Authorization: authorization(),
					SealosAccountAuthorization: accountAuthorization(),
					namespace: input.namespace,
					name: input.name,
					capacity: input.capacity,
					capacity_bytes: input.capacityBytes,
					access_modes: input.accessModes,
					storage_class_name: input.storageClassName ?? '',
				})
				return response.pvc
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async createViewerSession(input: CreateViewerSessionInput): Promise<ViewerSession> {
			try {
				const response = await activeClient.viewer.CreateViewerSession({
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

		async deletePVC(input: DeletePVCInput): Promise<PVC> {
			try {
				const response = await activeClient.viewer.DeletePVC(input.namespace, input.name, {
					Authorization: authorization(),
				})
				return response.pvc
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async expandPVC(input: ExpandPVCInput): Promise<PVC> {
			try {
				const response = await activeClient.viewer.ExpandPVC(input.namespace, input.name, {
					Authorization: authorization(),
					SealosAccountAuthorization: accountAuthorization(),
					capacity: input.capacity,
					capacity_bytes: input.capacityBytes,
				})
				return response.pvc
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async getContext(): Promise<ViewerContext> {
			try {
				const response = await activeClient.viewer.GetContext({
					Authorization: authorization(),
				})
				return response.context
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async getStorageQuota(input: GetStorageQuotaInput): Promise<StorageQuota> {
			try {
				const response = await activeClient.viewer.GetStorageQuota({
					Authorization: authorization(),
					Namespace: input.namespace,
					SealosAccountAuthorization: accountAuthorization(),
				})
				return response.storage_quota
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async getPodSession(podSessionID: string): Promise<PodSession> {
			try {
				const response = await activeClient.viewer.GetPodSession(podSessionID, {
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
				const response = await activeClient.viewer.GetViewerSession(viewerSessionID, {
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
				const response = await activeClient.viewer.HeartbeatViewerSession(viewerSessionID, {
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
				const response = await activeClient.viewer.IssueViewerToken(viewerSessionID, {
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
				const response = await activeClient.viewer.ListPVCs({
					Authorization: authorization(),
					Namespace: input.namespace,
				})
				return response.pvc_list.items
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async listStorageClasses(): Promise<StorageClass[]> {
			try {
				const response = await activeClient.viewer.ListStorageClasses({
					Authorization: authorization(),
				})
				return response.storage_class_list.items
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},
	}
}

export const viewerApi = createViewerApi()
