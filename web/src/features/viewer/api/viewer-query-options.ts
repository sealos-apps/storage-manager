import type { ViewerAPI } from '@/features/viewer/types/viewer'

import { queryOptions } from '@tanstack/react-query'

import { viewerApi } from '@/features/viewer/api/viewer-api'
import { isMissingSessionError } from '@/features/viewer/api/viewer-error'
import { viewerKeys } from '@/features/viewer/api/viewer-query-keys'

interface ViewerSessionQueryOptionsInput {
	api?: ViewerAPI
	enabled?: boolean
	viewerSessionID: string
}

export function pvcListQueryOptions(namespace: string, api: ViewerAPI = viewerApi) {
	return queryOptions({
		queryKey: viewerKeys.pvcs(namespace),
		queryFn: () => api.listPVCs({ namespace }),
		enabled: namespace.length > 0,
		staleTime: 15_000,
	})
}

export function viewerContextQueryOptions(api: ViewerAPI = viewerApi) {
	return queryOptions({
		queryKey: viewerKeys.context(),
		queryFn: () => api.getContext(),
		staleTime: 60_000,
	})
}

export function storageClassListQueryOptions(api: ViewerAPI = viewerApi) {
	return queryOptions({
		queryKey: viewerKeys.storageClasses(),
		queryFn: () => api.listStorageClasses(),
		staleTime: 60_000,
	})
}

export function storageQuotaQueryOptions(namespace: string, api: ViewerAPI = viewerApi) {
	return queryOptions({
		queryKey: viewerKeys.storageQuota(namespace),
		queryFn: () => api.getStorageQuota({ namespace }),
		enabled: namespace.length > 0,
		staleTime: 15_000,
	})
}

export function adminCapabilitiesQueryOptions(api: ViewerAPI = viewerApi) {
	return queryOptions({
		queryKey: viewerKeys.adminCapabilities(),
		queryFn: () => api.adminCapabilities(),
		staleTime: 60_000,
	})
}

export function adminNamespaceListQueryOptions(api: ViewerAPI = viewerApi, enabled = true) {
	return queryOptions({
		queryKey: viewerKeys.adminNamespaces(),
		queryFn: () => api.adminListNamespaces(),
		enabled,
		staleTime: 60_000,
	})
}

export function adminStorageClassListQueryOptions(api: ViewerAPI = viewerApi, enabled = true) {
	return queryOptions({
		queryKey: viewerKeys.adminStorageClasses(),
		queryFn: () => api.adminListStorageClasses(),
		enabled,
		staleTime: 15_000,
	})
}

export function adminStorageClassYAMLQueryOptions(name: string | null, api: ViewerAPI = viewerApi) {
	return queryOptions({
		queryKey: viewerKeys.adminStorageClassYAML(name ?? ''),
		queryFn: () => api.adminGetStorageClassYAML(name ?? ''),
		enabled: Boolean(name),
		staleTime: 5_000,
	})
}

export function adminStorageClassDescribeQueryOptions(name: string | null, api: ViewerAPI = viewerApi) {
	return queryOptions({
		queryKey: viewerKeys.adminStorageClassDescribe(name ?? ''),
		queryFn: () => api.adminDescribeStorageClass(name ?? ''),
		enabled: Boolean(name),
		staleTime: 5_000,
	})
}

export function pvcYAMLQueryOptions(pvc: { name: string, namespace: string } | null, api: ViewerAPI = viewerApi) {
	return queryOptions({
		queryKey: viewerKeys.pvcYAML(pvc?.namespace ?? '', pvc?.name ?? ''),
		queryFn: () => api.getPVCYAML({ namespace: pvc?.namespace ?? '', name: pvc?.name ?? '' }),
		enabled: Boolean(pvc),
		staleTime: 5_000,
	})
}

export function pvcDescribeQueryOptions(pvc: { name: string, namespace: string } | null, api: ViewerAPI = viewerApi) {
	return queryOptions({
		queryKey: viewerKeys.pvcDescribe(pvc?.namespace ?? '', pvc?.name ?? ''),
		queryFn: () => api.describePVC({ namespace: pvc?.namespace ?? '', name: pvc?.name ?? '' }),
		enabled: Boolean(pvc),
		staleTime: 5_000,
	})
}

export function viewerSessionQueryOptions({
	api = viewerApi,
	enabled = true,
	viewerSessionID,
}: ViewerSessionQueryOptionsInput) {
	return queryOptions({
		queryKey: viewerKeys.viewerSession(viewerSessionID),
		queryFn: () => api.getViewerSession(viewerSessionID),
		enabled: enabled && viewerSessionID.length > 0,
		refetchInterval: query =>
			query.state.data?.status === 'creating' ? 2_000 : false,
		retry: (_failureCount, error) => !isMissingSessionError(error),
		staleTime: 2_000,
	})
}

export function podSessionQueryOptions(
	podSessionID: string,
	api: ViewerAPI = viewerApi,
	enabled = true,
) {
	return queryOptions({
		queryKey: viewerKeys.podSession(podSessionID),
		queryFn: () => api.getPodSession(podSessionID),
		enabled: enabled && podSessionID.length > 0,
		staleTime: 5_000,
	})
}
