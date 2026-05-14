import type { QueryClient } from '@tanstack/react-query'
import type { CreateViewerSessionInput, ViewerAPI, ViewerSession } from '@/features/viewer/types/viewer'

import { mutationOptions } from '@tanstack/react-query'

import { viewerApi } from '@/features/viewer/api/viewer-api'
import { viewerKeys } from '@/features/viewer/api/viewer-query-keys'

export function createViewerSessionMutationOptions(
	queryClient: QueryClient,
	api: ViewerAPI = viewerApi,
) {
	return mutationOptions({
		mutationFn: (input: CreateViewerSessionInput) =>
			api.createViewerSession(input),
		onSuccess: (viewerSession) => {
			queryClient.setQueryData(
				viewerKeys.viewerSession(viewerSession.id),
				viewerSession,
			)
			void queryClient.invalidateQueries({ queryKey: viewerKeys.all })
		},
	})
}

export function issueViewerTokenMutationOptions(api: ViewerAPI = viewerApi) {
	return mutationOptions({
		mutationFn: (viewerSessionID: string) => api.issueViewerToken(viewerSessionID),
	})
}

export function heartbeatViewerSessionMutationOptions(
	queryClient: QueryClient,
	api: ViewerAPI = viewerApi,
) {
	return mutationOptions({
		mutationFn: (viewerSessionID: string) =>
			api.heartbeatViewerSession(viewerSessionID),
		onMutate: async (viewerSessionID) => {
			const key = viewerKeys.viewerSession(viewerSessionID)
			await queryClient.cancelQueries({ queryKey: key })
			const previous = queryClient.getQueryData<ViewerSession>(key)
			if (previous) {
				const now = new Date().toISOString()
				queryClient.setQueryData<ViewerSession>(key, {
					...previous,
					last_heartbeat_at: now,
				})
			}
			return { previous, viewerSessionID }
		},
		onError: (_error, _variables, context) => {
			if (context?.previous) {
				queryClient.setQueryData(
					viewerKeys.viewerSession(context.viewerSessionID),
					context.previous,
				)
			}
		},
	})
}

export function closeViewerSessionMutationOptions(
	queryClient: QueryClient,
	api: ViewerAPI = viewerApi,
) {
	return mutationOptions({
		mutationFn: (viewerSessionID: string) =>
			api.closeViewerSession(viewerSessionID),
		onMutate: async (viewerSessionID) => {
			const key = viewerKeys.viewerSession(viewerSessionID)
			await queryClient.cancelQueries({ queryKey: key })
			const previous = queryClient.getQueryData<ViewerSession>(key)
			if (previous) {
				queryClient.setQueryData<ViewerSession>(key, {
					...previous,
					status: 'closed',
				})
			}
			return { previous, viewerSessionID }
		},
		onError: (_error, _variables, context) => {
			if (context?.previous) {
				queryClient.setQueryData(
					viewerKeys.viewerSession(context.viewerSessionID),
					context.previous,
				)
			}
		},
		onSettled: (_data, _error, viewerSessionID) => {
			void queryClient.invalidateQueries({
				queryKey: viewerKeys.viewerSession(viewerSessionID),
			})
			void queryClient.invalidateQueries({ queryKey: viewerKeys.all })
		},
	})
}

export function closePodSessionMutationOptions(
	queryClient: QueryClient,
	api: ViewerAPI = viewerApi,
) {
	return mutationOptions({
		mutationFn: (podSessionID: string) => api.closePodSession(podSessionID),
		onSettled: (_data, _error, podSessionID) => {
			void queryClient.invalidateQueries({
				queryKey: viewerKeys.podSession(podSessionID),
			})
			void queryClient.invalidateQueries({ queryKey: viewerKeys.all })
		},
	})
}
