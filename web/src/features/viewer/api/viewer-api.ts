import type {
	AdminCapabilities,
	AdminNamespace,
	CreatePVCInput,
	CreateViewerSessionInput,
	DeletePVCInput,
	ExpandPVCInput,
	Heartbeat,
	ListPVCsInput,
	PodSession,
	PVC,
	StorageClass,
	StorageClassDescribe,
	StorageClassPolicyInput,
	StorageClassYAML,
	StorageClassYAMLInput,
	ViewerAPI,
	ViewerContext,
	ViewerSession,
	ViewerToken,
} from '@/features/viewer/types/viewer'

import Client, { Local } from '@sealos-storage-manager/encore-client'

import { env } from '@/config/env'
import { normalizeViewerError, ViewerApiError } from '@/features/viewer/api/viewer-error'

const kubeconfigStorageKey = 'sealos-storage-manager.kubeconfig'

export function readAuthorizationHeader() {
	const configured = import.meta.env.VITE_VIEWER_AUTHORIZATION
	if (configured) {
		return configured
	}
	const devKubeconfig = import.meta.env.VITE_DEV_KUBECONFIG
	if (devKubeconfig) {
		return `Bearer ${encodeURIComponent(devKubeconfig)}`
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
		async adminCapabilities(): Promise<AdminCapabilities> {
			try {
				const response = await client.viewer.AdminCapabilities({
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
				const response = await client.viewer.AdminCreateStorageClass({
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
				const response = await client.viewer.AdminDeleteStorageClass(name, {
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
				const response = await client.viewer.AdminDescribeStorageClass(name, {
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
				const response = await client.viewer.AdminGetStorageClassYAML(name, {
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
				const response = await client.viewer.AdminListNamespaces({
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
				const response = await client.viewer.AdminListStorageClasses({
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
				const response = await client.viewer.AdminUpdateStorageClass(name, {
					Authorization: authorization(),
					yaml: input.yaml,
				})
				return response.storage_class
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

		async adminUpdateStorageClassPolicy(name: string, input: StorageClassPolicyInput): Promise<StorageClass> {
			try {
				const response = await client.viewer.AdminUpdateStorageClassPolicy(name, {
					Authorization: authorization(),
					visible_in_create: input.visibleInCreate,
					allowed_access_modes: input.allowedAccessModes,
				})
				return response.storage_class
			}
			catch (error) {
				throw normalizeViewerError(error)
			}
		},

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

		async createPVC(input: CreatePVCInput): Promise<PVC> {
			try {
				const response = await client.viewer.CreatePVC({
					Authorization: authorization(),
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

		async deletePVC(input: DeletePVCInput): Promise<PVC> {
			try {
				const response = await client.viewer.DeletePVC(input.namespace, input.name, {
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
				const response = await client.viewer.ExpandPVC(input.namespace, input.name, {
					Authorization: authorization(),
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
				const response = await client.viewer.GetContext({
					Authorization: authorization(),
				})
				return response.context
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

		async listStorageClasses(): Promise<StorageClass[]> {
			try {
				const response = await client.viewer.ListStorageClasses({
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
