import type { FileBrowserSession, FileListResult, FileUsage, RecycleEntry } from '@/features/file-manager/types/file-manager'

import type { FileSortState } from '@/features/file-manager/utils/file-tree'

import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { fileManagerKeys } from '@/features/file-manager/api/file-manager-query-keys'
import { readRecycleIndex } from '@/features/file-manager/api/recycle-bin-api'
import { flattenResources, sortEntries } from '@/features/file-manager/utils/file-tree'

export function fileListQueryOptions(
	session: FileBrowserSession | null,
	path: string,
	sort: FileSortState,
	enabled = true,
) {
	const sortKey = `${sort.field}:${sort.direction}`
	return queryOptions({
		queryKey: fileManagerKeys.files(session?.pvcKey ?? 'inactive', path, sortKey),
		queryFn: async ({ signal }): Promise<FileListResult> => {
			if (!session) {
				throw new Error('File Browser session is not ready')
			}
			const current = await session.client.list(path, signal)
			return {
				current,
				entries: sortEntries(flattenResources(current), sort),
				path,
			}
		},
		enabled: session !== null && enabled,
		placeholderData: keepPreviousData,
		staleTime: 5_000,
	})
}

export function fileUsageQueryOptions(session: FileBrowserSession | null, enabled = true) {
	return queryOptions({
		queryKey: fileManagerKeys.usage(session?.pvcKey ?? 'inactive'),
		queryFn: ({ signal }): Promise<FileUsage> => {
			if (!session) {
				throw new Error('File Browser session is not ready')
			}
			return session.client.usage('/', signal)
		},
		enabled: session !== null && enabled,
		staleTime: 5_000,
	})
}

export function fileTextQueryOptions(
	session: FileBrowserSession | null,
	path: string | null,
) {
	return queryOptions({
		queryKey: fileManagerKeys.text(session?.pvcKey ?? 'inactive', path ?? ''),
		queryFn: ({ signal }) => {
			if (!session || !path) {
				throw new Error('File Browser session is not ready')
			}
			return session.client.readText(path, signal)
		},
		enabled: session !== null && path !== null,
		staleTime: 0,
	})
}

export function recycleBinQueryOptions(session: FileBrowserSession | null) {
	return queryOptions({
		queryKey: fileManagerKeys.recycleBin(session?.pvcKey ?? 'inactive'),
		queryFn: async (): Promise<RecycleEntry[]> => {
			if (!session) {
				throw new Error('File Browser session is not ready')
			}
			return readRecycleIndex(session.client)
		},
		enabled: session !== null,
		staleTime: 5_000,
	})
}
