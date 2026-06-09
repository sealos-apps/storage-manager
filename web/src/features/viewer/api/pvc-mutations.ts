import type { QueryClient } from '@tanstack/react-query'
import type { CreatePVCInput, DeletePVCInput, ExpandPVCInput, PVC, ViewerAPI } from '@/features/viewer/types/viewer'

import { mutationOptions } from '@tanstack/react-query'

import { viewerApi } from '@/features/viewer/api/viewer-api'
import { viewerKeys } from '@/features/viewer/api/viewer-query-keys'

export function createPVCMutationOptions(
	queryClient: QueryClient,
	api: ViewerAPI = viewerApi,
) {
	return mutationOptions({
		mutationKey: viewerKeys.mutations.createPVC(),
		mutationFn: (input: CreatePVCInput) => api.createPVC(input),
		onMutate: async (input) => {
			const key = viewerKeys.pvcs(input.namespace)
			await queryClient.cancelQueries({ queryKey: key })
			const previous = queryClient.getQueryData<PVC[]>(key)
			const optimistic: PVC = {
				namespace: input.namespace,
				name: input.name,
				uid: `optimistic:${input.namespace}:${input.name}`,
				capacity_bytes: input.capacityBytes,
				capacity: input.capacity,
				access_modes: input.accessModes,
				storage_class_name: input.storageClassName,
				mounted: false,
				mounted_pods: [],
				viewer_supported: true,
				viewer_mode: 'readwrite',
				viewer_scheduling: {
					requires_node: false,
					node_name: '',
					reason: '',
				},
				reason: '',
			}
			queryClient.setQueryData<PVC[]>(key, current => [
				...(current ?? []).filter(pvc => pvc.name !== input.name),
				optimistic,
			])
			return { key, previous }
		},
		onError: (_error, _variables, context) => {
			if (context?.previous) {
				queryClient.setQueryData(context.key, context.previous)
			}
		},
		onSuccess: (pvc, variables) => {
			queryClient.setQueryData<PVC[]>(
				viewerKeys.pvcs(variables.namespace),
				current => (current ?? []).map(item =>
					item.name === variables.name ? pvc : item,
				),
			)
		},
		onSettled: (_data, _error, variables) => {
			void queryClient.invalidateQueries({
				queryKey: viewerKeys.pvcs(variables.namespace),
			})
		},
	})
}

export function deletePVCMutationOptions(
	queryClient: QueryClient,
	api: ViewerAPI = viewerApi,
) {
	return mutationOptions({
		mutationKey: viewerKeys.mutations.deletePVC(),
		mutationFn: (input: DeletePVCInput) => api.deletePVC(input),
		onMutate: async (input) => {
			const key = viewerKeys.pvcs(input.namespace)
			await queryClient.cancelQueries({ queryKey: key })
			const previous = queryClient.getQueryData<PVC[]>(key)
			queryClient.setQueryData<PVC[]>(
				key,
				current => (current ?? []).filter(pvc => pvc.name !== input.name),
			)
			return { key, previous }
		},
		onError: (_error, _variables, context) => {
			if (context?.previous) {
				queryClient.setQueryData(context.key, context.previous)
			}
		},
		onSettled: (_data, _error, variables) => {
			void queryClient.invalidateQueries({
				queryKey: viewerKeys.pvcs(variables.namespace),
			})
		},
	})
}

export function expandPVCMutationOptions(
	queryClient: QueryClient,
	api: ViewerAPI = viewerApi,
) {
	return mutationOptions({
		mutationKey: viewerKeys.mutations.expandPVC(),
		mutationFn: (input: ExpandPVCInput) => api.expandPVC(input),
		onMutate: async (input) => {
			const key = viewerKeys.pvcs(input.namespace)
			await queryClient.cancelQueries({ queryKey: key })
			const previous = queryClient.getQueryData<PVC[]>(key)
			queryClient.setQueryData<PVC[]>(
				key,
				current => (current ?? []).map(pvc =>
					pvc.name === input.name
						? {
								...pvc,
								capacity: input.capacity,
								capacity_bytes: input.capacityBytes,
							}
						: pvc,
				),
			)
			return { key, previous }
		},
		onError: (_error, _variables, context) => {
			if (context?.previous) {
				queryClient.setQueryData(context.key, context.previous)
			}
		},
		onSuccess: (pvc, variables) => {
			queryClient.setQueryData<PVC[]>(
				viewerKeys.pvcs(variables.namespace),
				current => (current ?? []).map(item =>
					item.name === variables.name ? pvc : item,
				),
			)
		},
		onSettled: (_data, _error, variables) => {
			void queryClient.invalidateQueries({
				queryKey: viewerKeys.pvcs(variables.namespace),
			})
		},
	})
}
