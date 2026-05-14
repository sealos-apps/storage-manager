import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'

import { createViewerSessionMutationOptions, heartbeatViewerSessionMutationOptions } from '@/features/viewer/api/viewer-mutations'
import { viewerKeys } from '@/features/viewer/api/viewer-query-keys'
import { pvcListQueryOptions, viewerSessionQueryOptions } from '@/features/viewer/api/viewer-query-options'
import { createFakeViewerAPI } from '@/features/viewer/test/fakes'

const mutationContext = {
	client: new QueryClient(),
	meta: undefined,
}

describe('viewer query options', () => {
	it('uses stable namespace-specific PVC query keys', async () => {
		const api = createFakeViewerAPI()
		const options = pvcListQueryOptions('default', api)

		expect(options.queryKey).toEqual(viewerKeys.pvcs('default'))
		await expect(options.queryFn?.({
			client: mutationContext.client,
			meta: undefined,
			queryKey: options.queryKey,
			signal: new AbortController().signal,
		})).resolves.toHaveLength(1)
	})

	it('polls viewer sessions only while creating', () => {
		const options = viewerSessionQueryOptions({
			api: createFakeViewerAPI(),
			viewerSessionID: 'vs_1',
		})
		const refetchInterval = options.refetchInterval as (query: { state: { data?: { status: string } } }) => false | number

		expect(options.queryKey).toEqual(viewerKeys.viewerSession('vs_1'))
		expect(refetchInterval({ state: { data: { status: 'creating' } } })).toBe(2000)
		expect(refetchInterval({ state: { data: { status: 'ready' } } })).toBe(false)
	})

	it('stores created viewer sessions and invalidates viewer queries', async () => {
		const queryClient = new QueryClient()
		const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
		const options = createViewerSessionMutationOptions(queryClient, createFakeViewerAPI())
		const input = {
			namespace: 'default',
			pvcName: 'data',
		}
		const session = await options.mutationFn?.(input, mutationContext)

		if (!session) {
			throw new Error('missing session')
		}
		await options.onSuccess?.(session, input, undefined, mutationContext)

		expect(queryClient.getQueryData(viewerKeys.viewerSession(session.id))).toEqual(session)
		expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: viewerKeys.all })
	})

	it('optimistically records heartbeat timestamps and rolls back on error', async () => {
		const queryClient = new QueryClient()
		const key = viewerKeys.viewerSession('vs_1')
		queryClient.setQueryData(key, {
			id: 'vs_1',
			last_heartbeat_at: 'old',
		})

		const options = heartbeatViewerSessionMutationOptions(queryClient, createFakeViewerAPI())
		const context = await options.onMutate?.('vs_1', mutationContext)

		expect(queryClient.getQueryData<{ last_heartbeat_at: string }>(key)?.last_heartbeat_at).not.toBe('old')

		options.onError?.(new Error('failed'), 'vs_1', context, mutationContext)

		expect(queryClient.getQueryData<{ last_heartbeat_at: string }>(key)?.last_heartbeat_at).toBe('old')
	})
})
