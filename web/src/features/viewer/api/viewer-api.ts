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
	PVCDescribe,
	PVCYAML,
	RawPVC,
	RawStorageQuota,
	StorageClass,
	StorageClassDescribe,
	StorageClassMetadataInput,
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
import { quantityFromBytes, quantityToSafeBytes } from '@/features/viewer/utils/storage-quantity'
import { getCachedAccountAuthorizationHeader, getCachedAuthorizationHeader } from '@/services/sealos/sealos-authorization'
import { Quantity } from '@/utils/quantities'

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

function mapPVC(pvc: RawPVC): PVC {
	const capacity = (pvc as unknown as { capacity?: Quantity | string }).capacity
	const stats = pvc.volume_stats as RawPVC['volume_stats'] & Partial<PVC['volume_stats']> | undefined
	return {
		...pvc,
		capacity: capacity instanceof Quantity
			? capacity
			: pvc.capacity ? Quantity.parse(pvc.capacity) : quantityFromBytes(pvc.capacity_bytes),
		volume_stats: stats
			? {
					...stats,
					available: stats.available instanceof Quantity ? stats.available : quantityFromBytes(stats.available_bytes ?? 0),
					metricCapacity: stats.metricCapacity instanceof Quantity ? stats.metricCapacity : quantityFromBytes(stats.metric_capacity_bytes ?? 0),
					used: stats.used instanceof Quantity ? stats.used : quantityFromBytes(stats.used_bytes ?? 0),
				}
			: undefined,
	}
}

function mapQuota(quota: RawStorageQuota): StorageQuota {
	const hydrated = quota as RawStorageQuota & Partial<StorageQuota>
	return {
		...quota,
		available: hydrated.available instanceof Quantity ? hydrated.available : quota.available_quantity ? Quantity.parse(quota.available_quantity) : quantityFromBytes(quota.available_bytes),
		limit: hydrated.limit instanceof Quantity ? hydrated.limit : quota.limit_quantity ? Quantity.parse(quota.limit_quantity) : quantityFromBytes(quota.limit_bytes),
		used: hydrated.used instanceof Quantity ? hydrated.used : quota.used_quantity ? Quantity.parse(quota.used_quantity) : quantityFromBytes(quota.used_bytes),
	}
}

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

		async adminUpdateStorageClassMetadata(name: string, input: StorageClassMetadataInput): Promise<StorageClass> {
			try {
				const response = await activeClient.viewer.AdminUpdateStorageClassMetadata(name, {
					Authorization: authorization(),
					available_to_users: input.availableToUsers,
					display_names: input.displayNames,
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
				const capacityBytes = quantityToSafeBytes(input.capacity)
				if (capacityBytes === null) {
					throw new Error('PVC capacity is outside JavaScript safe integer range.')
				}
				const response = await activeClient.viewer.CreatePVC({
					Authorization: authorization(),
					SealosAccountAuthorization: accountAuthorization(),
					namespace: input.namespace,
					name: input.name,
					capacity: input.capacity.toString(),
					capacity_bytes: capacityBytes,
					access_modes: input.accessModes,
					storage_class_name: input.storageClassName ?? '',
				})
				return mapPVC(response.pvc)
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
				return mapPVC(response.pvc)
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async expandPVC(input: ExpandPVCInput): Promise<PVC> {
			try {
				const capacityBytes = quantityToSafeBytes(input.capacity)
				if (capacityBytes === null) {
					throw new Error('PVC capacity is outside JavaScript safe integer range.')
				}
				const response = await activeClient.viewer.ExpandPVC(input.namespace, input.name, {
					Authorization: authorization(),
					SealosAccountAuthorization: accountAuthorization(),
					capacity: input.capacity.toString(),
					capacity_bytes: capacityBytes,
				})
				return mapPVC(response.pvc)
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
				return mapQuota(response.storage_quota)
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
				return response.pvc_list.items.map(mapPVC)
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

		async describePVC(input: DeletePVCInput): Promise<PVCDescribe> {
			try {
				const response = await activeClient.viewer.DescribePVC(input.namespace, input.name, {
					Authorization: authorization(),
				})
				return response.pvc_describe
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async getPVCYAML(input: DeletePVCInput): Promise<PVCYAML> {
			try {
				const response = await activeClient.viewer.GetPVCYAML(input.namespace, input.name, {
					Authorization: authorization(),
				})
				return response.pvc_yaml
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async updatePVC(input: DeletePVCInput & StorageClassYAMLInput): Promise<PVC> {
			try {
				const response = await activeClient.viewer.UpdatePVC(input.namespace, input.name, {
					Authorization: authorization(),
					SealosAccountAuthorization: accountAuthorization(),
					yaml: input.yaml,
				})
				return mapPVC(response.pvc)
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},
	}
}

export const viewerApi = createViewerApi()
