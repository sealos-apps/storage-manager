export const fileManagerKeys = {
	all: ['file-manager'] as const,
	fileLists: (pvcKey: string) => [...fileManagerKeys.all, pvcKey, 'files'] as const,
	files: (pvcKey: string, path: string, sortKey = '') =>
		[...fileManagerKeys.fileLists(pvcKey), path, sortKey] as const,
	usage: (pvcKey: string) => [...fileManagerKeys.all, pvcKey, 'usage'] as const,
	mutations: {
		clearRecycleBin: (pvcKey: string) =>
			[...fileManagerKeys.all, pvcKey, 'mutation', 'clear-recycle-bin'] as const,
		createFolder: (pvcKey: string) =>
			[...fileManagerKeys.all, pvcKey, 'mutation', 'create-folder'] as const,
		moveToRecycleBin: (pvcKey: string) =>
			[...fileManagerKeys.all, pvcKey, 'mutation', 'move-to-recycle-bin'] as const,
		restoreRecycleEntry: (pvcKey: string) =>
			[...fileManagerKeys.all, pvcKey, 'mutation', 'restore-recycle-entry'] as const,
		saveText: (pvcKey: string) =>
			[...fileManagerKeys.all, pvcKey, 'mutation', 'save-text'] as const,
		uploadFile: (pvcKey: string) =>
			[...fileManagerKeys.all, pvcKey, 'mutation', 'upload-file'] as const,
	},
	recycleBin: (pvcKey: string) =>
		[...fileManagerKeys.all, pvcKey, 'recycle-bin'] as const,
	text: (pvcKey: string, path: string) =>
		[...fileManagerKeys.all, pvcKey, 'text', path] as const,
	texts: (pvcKey: string) => [...fileManagerKeys.all, pvcKey, 'text'] as const,
}
