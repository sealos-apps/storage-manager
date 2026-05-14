import type { ViewerAPI } from '@/features/viewer/types/viewer'

import { queryOptions } from '@tanstack/react-query'

import { viewerApi } from '@/features/viewer/api/viewer-api'
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
		staleTime: 15_000,
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
