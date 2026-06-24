import type { QueryClient } from '@tanstack/react-query'
import type { StorageClass, StorageClassMetadataInput, StorageClassYAMLInput, ViewerAPI } from '@/features/viewer/types/viewer'

import { mutationOptions } from '@tanstack/react-query'

import { viewerApi } from '@/features/viewer/api/viewer-api'
import { viewerKeys } from '@/features/viewer/api/viewer-query-keys'

export function adminCreateStorageClassMutationOptions(
	queryClient: QueryClient,
	api: ViewerAPI = viewerApi,
) {
	return mutationOptions({
		mutationKey: viewerKeys.mutations.adminCreateStorageClass(),
		mutationFn: (input: StorageClassYAMLInput) => api.adminCreateStorageClass(input),
		onSuccess: () => {
			void queryClient.invalidateQueries({ queryKey: viewerKeys.adminStorageClasses() })
			void queryClient.invalidateQueries({ queryKey: viewerKeys.storageClasses() })
		},
	})
}

export function adminUpdateStorageClassMutationOptions(
	queryClient: QueryClient,
	api: ViewerAPI = viewerApi,
) {
	return mutationOptions({
		mutationKey: viewerKeys.mutations.adminUpdateStorageClass(),
		mutationFn: (input: { name: string } & StorageClassYAMLInput) =>
			api.adminUpdateStorageClass(input.name, { yaml: input.yaml }),
		onSuccess: (storageClass: StorageClass) => {
			queryClient.setQueryData<StorageClass[]>(
				viewerKeys.adminStorageClasses(),
				current => (current ?? []).map(item => item.name === storageClass.name ? storageClass : item),
			)
			void queryClient.invalidateQueries({ queryKey: viewerKeys.adminStorageClassYAML(storageClass.name) })
			void queryClient.invalidateQueries({ queryKey: viewerKeys.adminStorageClasses() })
			void queryClient.invalidateQueries({ queryKey: viewerKeys.storageClasses() })
		},
	})
}

export function adminDeleteStorageClassMutationOptions(
	queryClient: QueryClient,
	api: ViewerAPI = viewerApi,
) {
	return mutationOptions({
		mutationKey: viewerKeys.mutations.adminDeleteStorageClass(),
		mutationFn: (name: string) => api.adminDeleteStorageClass(name),
		onSuccess: (storageClass) => {
			queryClient.setQueryData<StorageClass[]>(
				viewerKeys.adminStorageClasses(),
				current => (current ?? []).filter(item => item.name !== storageClass.name),
			)
			void queryClient.invalidateQueries({ queryKey: viewerKeys.storageClasses() })
		},
	})
}

export function adminUpdateStorageClassMetadataMutationOptions(
	queryClient: QueryClient,
	api: ViewerAPI = viewerApi,
) {
	return mutationOptions({
		mutationKey: viewerKeys.mutations.adminUpdateStorageClassMetadata(),
		mutationFn: (input: { name: string } & StorageClassMetadataInput) =>
			api.adminUpdateStorageClassMetadata(input.name, {
				availableToUsers: input.availableToUsers,
				displayNames: input.displayNames,
			}),
		onSuccess: (storageClass: StorageClass) => {
			queryClient.setQueryData<StorageClass[]>(
				viewerKeys.adminStorageClasses(),
				current => (current ?? []).map(item => item.name === storageClass.name ? storageClass : item),
			)
			void queryClient.invalidateQueries({ queryKey: viewerKeys.adminStorageClasses() })
			void queryClient.invalidateQueries({ queryKey: viewerKeys.storageClasses() })
		},
	})
}
