import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'

import {
	adminCreateStorageClassMutationOptions,
	adminDeleteStorageClassMutationOptions,
	adminUpdateStorageClassMutationOptions,
	closePodSessionMutationOptions,
	closeViewerSessionMutationOptions,
	createPVCMutationOptions,
	deletePVCMutationOptions,
	expandPVCMutationOptions,
} from '@/features/viewer/api/viewer-mutations'
import { viewerKeys } from '@/features/viewer/api/viewer-query-keys'
import { createFakeViewerAPI, pvcFixture, storageClassFixture, viewerSessionFixture } from '@/features/viewer/test/fakes'
import { Quantity } from '@/utils/quantities'

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

	it('optimistically creates PVCs and replaces them with server data', async () => {
		const queryClient = new QueryClient()
		const key = viewerKeys.pvcs('default')
		queryClient.setQueryData(key, [pvcFixture({ name: 'existing' })])
		const options = createPVCMutationOptions(queryClient, createFakeViewerAPI())
		const input = {
			namespace: 'default',
			name: 'cache-data',
			capacity: Quantity.parse('5Gi'),
			accessModes: ['ReadWriteOnce'],
			storageClassName: 'standard',
		}

		const context = await options.onMutate?.(input, mutationContext)

		expect(queryClient.getQueryData(key)).toEqual(expect.arrayContaining([
			expect.objectContaining({ name: 'cache-data', uid: expect.stringContaining('optimistic') }),
		]))

		if (!context) {
			throw new Error('missing mutation context')
		}
		await options.onSuccess?.(pvcFixture({ name: 'cache-data', uid: 'server-uid' }), input, context, mutationContext)

		expect(queryClient.getQueryData(key)).toEqual(expect.arrayContaining([
			expect.objectContaining({ name: 'cache-data', uid: 'server-uid' }),
		]))
	})

	it('optimistically deletes PVCs and rolls back on error', async () => {
		const queryClient = new QueryClient()
		const key = viewerKeys.pvcs('default')
		const original = [pvcFixture({ name: 'mysql-data' })]
		queryClient.setQueryData(key, original)
		const options = deletePVCMutationOptions(queryClient, createFakeViewerAPI())
		const input = { namespace: 'default', name: 'mysql-data' }

		const context = await options.onMutate?.(input, mutationContext)

		expect(queryClient.getQueryData<typeof original>(key)).toEqual([])

		options.onError?.(new Error('failed'), input, context, mutationContext)

		expect(queryClient.getQueryData(key)).toEqual(original)
	})

	it('invalidates user and admin StorageClass lists after admin mutations', async () => {
		const queryClient = new QueryClient()
		const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
		const api = createFakeViewerAPI()

		adminCreateStorageClassMutationOptions(queryClient, api).onSuccess?.(
			storageClassFixture({ name: 'created' }),
			{ yaml: 'kind: StorageClass' },
			undefined,
			mutationContext,
		)

		expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: viewerKeys.adminStorageClasses() })
		expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: viewerKeys.storageClasses() })
	})

	it('updates and removes admin StorageClass cache entries', () => {
		const queryClient = new QueryClient()
		queryClient.setQueryData(viewerKeys.adminStorageClasses(), [
			{ name: 'standard' },
			{ name: 'shared' },
		])

		adminUpdateStorageClassMutationOptions(queryClient, createFakeViewerAPI()).onSuccess?.(
			{ name: 'standard', provisioner: 'updated' } as never,
			{ name: 'standard', yaml: 'kind: StorageClass' },
			undefined,
			mutationContext,
		)
		expect(queryClient.getQueryData(viewerKeys.adminStorageClasses())).toEqual(expect.arrayContaining([
			expect.objectContaining({ name: 'standard', provisioner: 'updated' }),
		]))

		adminDeleteStorageClassMutationOptions(queryClient, createFakeViewerAPI()).onSuccess?.(
			{ name: 'standard' } as never,
			'standard',
			undefined,
			mutationContext,
		)
		expect(queryClient.getQueryData(viewerKeys.adminStorageClasses())).toEqual([
			expect.objectContaining({ name: 'shared' }),
		])
	})

	it('optimistically expands PVCs and rolls back on error', async () => {
		const queryClient = new QueryClient()
		const key = viewerKeys.pvcs('default')
		const original = [pvcFixture({ name: 'mysql-data', capacity: '10Gi' })]
		queryClient.setQueryData(key, original)
		const options = expandPVCMutationOptions(queryClient, createFakeViewerAPI())
		const input = {
			namespace: 'default',
			name: 'mysql-data',
			capacity: Quantity.parse('20Gi'),
		}

		const context = await options.onMutate?.(input, mutationContext)

		expect(queryClient.getQueryData<typeof original>(key)?.[0]?.capacity.toString()).toBe('20Gi')

		options.onError?.(new Error('failed'), input, context, mutationContext)

		expect(queryClient.getQueryData(key)).toEqual(original)
	})
})
