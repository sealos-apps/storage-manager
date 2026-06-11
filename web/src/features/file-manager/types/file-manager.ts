import type { FileBrowserClient, FileBrowserResource } from '@sealos-storage-manager/filebrowser-client'

export type FileEntryType = 'directory' | 'file'

export interface FileEntry {
	depth: number
	isDir: boolean
	modified: string
	name: string
	path: string
	size: number
	type: FileEntryType
}

export type FileTableRow
	= | {
		entry: FileEntry
		id: string
		kind: 'resource'
	}
	| {
		depth: number
		error: Error
		id: string
		kind: 'branch-error'
		path: string
	}

export interface RecycleEntry {
	deletedAt: string
	id: string
	isDir: boolean
	name: string
	originalPath: string
	size: number
	trashPath: string
}

export interface FileBrowserSession {
	client: FileBrowserClient
	pvcKey: string
}

export interface FileListResult {
	current: FileBrowserResource
	entries: FileEntry[]
	path: string
}
