import type { FileBrowserSession } from '@/features/file-manager/types/file-manager'

import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'

import { invalidateFileManagerAfterMutation, invalidateFileTreeQueries } from '@/features/file-manager/api/file-manager-cache'
import { fileManagerKeys } from '@/features/file-manager/api/file-manager-query-keys'

describe('file manager cache', () => {
	it('invalidates parent, exact, and descendant file tree queries', () => {
		const queryClient = new QueryClient()
		const session = { pvcKey: 'pvc-1' } as FileBrowserSession
		queryClient.setQueryData(fileManagerKeys.files('pvc-1', '/', 'name:asc'), 'root')
		queryClient.setQueryData(fileManagerKeys.files('pvc-1', '/docs', 'name:asc'), 'docs')
		queryClient.setQueryData(fileManagerKeys.files('pvc-1', '/docs/nested', 'name:asc'), 'nested')
		queryClient.setQueryData(fileManagerKeys.files('pvc-1', '/other', 'name:asc'), 'other')

		invalidateFileTreeQueries(queryClient, session, ['/docs'])

		expect(queryClient.getQueryState(fileManagerKeys.files('pvc-1', '/', 'name:asc'))?.isInvalidated).toBe(true)
		expect(queryClient.getQueryState(fileManagerKeys.files('pvc-1', '/docs', 'name:asc'))?.isInvalidated).toBe(true)
		expect(queryClient.getQueryState(fileManagerKeys.files('pvc-1', '/docs/nested', 'name:asc'))?.isInvalidated).toBe(true)
		expect(queryClient.getQueryState(fileManagerKeys.files('pvc-1', '/other', 'name:asc'))?.isInvalidated).toBe(false)
	})

	it('invalidates file tree and recycle bin after file mutations', () => {
		const queryClient = new QueryClient()
		const session = { pvcKey: 'pvc-1' } as FileBrowserSession
		queryClient.setQueryData(fileManagerKeys.files('pvc-1', '/docs', 'name:asc'), 'docs')
		queryClient.setQueryData(fileManagerKeys.recycleBin('pvc-1'), [])

		invalidateFileManagerAfterMutation(queryClient, session, ['/docs/file.txt'])

		expect(queryClient.getQueryState(fileManagerKeys.files('pvc-1', '/docs', 'name:asc'))?.isInvalidated).toBe(true)
		expect(queryClient.getQueryState(fileManagerKeys.recycleBin('pvc-1'))?.isInvalidated).toBe(true)
	})
})
