import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'

import { closePodSessionMutationOptions, closeViewerSessionMutationOptions } from '@/features/viewer/api/viewer-mutations'
import { viewerKeys } from '@/features/viewer/api/viewer-query-keys'
import { createFakeViewerAPI, viewerSessionFixture } from '@/features/viewer/test/fakes'

const mutationContext = {
	client: new QueryClient(),
	meta: undefined,
}

describe('viewer mutation options', () => {
	it('optimistically closes viewer sessions and rolls back on error', async () => {
		const queryClient = new QueryClient()
		const key = viewerKeys.viewerSession('vs_1')
		const session = viewerSessionFixture({ id: 'vs_1', status: 'ready' })
		queryClient.setQueryData(key, session)
		const options = closeViewerSessionMutationOptions(queryClient, createFakeViewerAPI())

		const context = await options.onMutate?.('vs_1', mutationContext)

		expect(queryClient.getQueryData<typeof session>(key)?.status).toBe('closed')

		options.onError?.(new Error('failed'), 'vs_1', context, mutationContext)

		expect(queryClient.getQueryData<typeof session>(key)?.status).toBe('ready')
	})

	it('invalidates pod and viewer queries after closing pod sessions', () => {
		const queryClient = new QueryClient()
		const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
		const options = closePodSessionMutationOptions(queryClient, createFakeViewerAPI())

		options.onSettled?.(undefined, null, 'ps_1', undefined, mutationContext)

		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: viewerKeys.podSession('ps_1'),
		})
		expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: viewerKeys.all })
	})
})
