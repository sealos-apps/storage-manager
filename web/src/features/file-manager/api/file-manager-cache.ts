import type { QueryClient } from '@tanstack/react-query'
import type { FileBrowserSession } from '@/features/file-manager/types/file-manager'

import { parentPath } from '@sealos-storage-manager/filebrowser-client'

import { fileManagerKeys } from '@/features/file-manager/api/file-manager-query-keys'
import { trashRootPath } from '@/features/file-manager/utils/file-tree'

export function invalidateFileTreeQueries(
	queryClient: QueryClient,
	session: FileBrowserSession,
	paths: string[],
) {
	void queryClient.invalidateQueries({
		queryKey: fileManagerKeys.fileLists(session.pvcKey),
		predicate: (query) => {
			const queryPath = query.queryKey[3]
			if (typeof queryPath !== 'string') {
				return true
			}
			return paths.some(path =>
				queryPath === path
				|| queryPath === parentPath(path)
				|| queryPath.startsWith(`${path}/`)
				|| path.startsWith(`${queryPath}/`),
			)
		},
	})
}

export function invalidateFileManagerAfterMutation(
	queryClient: QueryClient,
	session: FileBrowserSession,
	paths: string[],
) {
	invalidateFileTreeQueries(queryClient, session, paths.filter(path => path !== trashRootPath))
	void queryClient.invalidateQueries({
		queryKey: fileManagerKeys.recycleBin(session.pvcKey),
	})
}
