import type { FileBrowserSession } from '@/features/file-manager/types/file-manager'

import { FileBrowserError } from '@sealos-storage-manager/filebrowser-client'
import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'

import {
	createFolderMutationOptions,
	moveToRecycleBinMutationOptions,
	saveFileTextMutationOptions,
	shouldReportUploadProgress,
	uploadFileMutationOptions,
	uploadFilesMutationOptions,
} from '@/features/file-manager/api/file-manager-mutations'
import { fileManagerKeys } from '@/features/file-manager/api/file-manager-query-keys'
import { uploadActions, uploadStore } from '@/features/file-manager/stores/upload-store'

const originalUpdateTask = uploadActions.updateTask

function fileBrowserError(status: number) {
	return new FileBrowserError({
		status,
		code: status === 409 ? 'FILE_CONFLICT' : 'FILEBROWSER_REQUEST_FAILED',
		message: 'exists',
	})
}

const mutationContext = {
	client: new QueryClient(),
	meta: undefined,
}

function createSession(overrides: Partial<FileBrowserSession['client']> = {}): FileBrowserSession {
	return {
		pvcKey: 'pvc-1',
		client: {
			createFolder: vi.fn().mockResolvedValue(undefined),
			move: vi.fn().mockResolvedValue(undefined),
			readText: vi.fn().mockResolvedValue(''),
			saveText: vi.fn().mockResolvedValue(undefined),
			uploadFile: vi.fn().mockResolvedValue(undefined),
			writeText: vi.fn().mockResolvedValue(undefined),
			...overrides,
		},
	} as unknown as FileBrowserSession
}

describe('file manager mutation options', () => {
	it('creates folders through a stable mutation key and invalidates file lists', async () => {
		const queryClient = new QueryClient()
		const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
		const session = createSession()
		const options = createFolderMutationOptions(queryClient, session)

		const result = await options.mutationFn?.({ currentPath: '/', name: 'test' }, mutationContext)
		await options.onSuccess?.(result!, { currentPath: '/', name: 'test' }, undefined, mutationContext)

		expect(options.mutationKey).toEqual(fileManagerKeys.mutations.createFolder('pvc-1'))
		expect(session.client.createFolder).toHaveBeenCalledWith('/test')
		expect(invalidateSpy).toHaveBeenCalledWith(expect.objectContaining({
			queryKey: fileManagerKeys.fileLists('pvc-1'),
		}))
	})

	it('moves entries to recycle bin and invalidates recycle and file list queries', async () => {
		const queryClient = new QueryClient()
		const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
		const session = createSession({
			createFolder: vi.fn().mockResolvedValue(undefined),
			readText: vi.fn().mockRejectedValue(Object.assign(new Error('not found'), { status: 404 })),
		})
		const options = moveToRecycleBinMutationOptions(queryClient, session)

		const entry = await options.mutationFn?.({ path: '/test', isDir: true, size: 0 }, mutationContext)
		await options.onSuccess?.(entry!, { path: '/test', isDir: true, size: 0 }, undefined, mutationContext)

		expect(session.client.move).toHaveBeenCalledWith('/test', expect.stringContaining('/.storage-manager-trash/objects/'), true)
		expect(session.client.writeText).toHaveBeenCalledWith(
			'/.storage-manager-trash/index.json',
			expect.stringContaining('"originalPath": "/test"'),
			true,
		)
		expect(session.client.saveText).not.toHaveBeenCalled()
		expect(invalidateSpy).toHaveBeenCalledWith(expect.objectContaining({
			queryKey: fileManagerKeys.recycleBin('pvc-1'),
		}))
	})

	it('treats existing File Browser trash folders as ready before moving files', async () => {
		const queryClient = new QueryClient()
		const createFolder = vi.fn()
			.mockRejectedValueOnce(fileBrowserError(409))
			.mockRejectedValueOnce(fileBrowserError(409))
			.mockRejectedValueOnce(fileBrowserError(409))
			.mockRejectedValueOnce(fileBrowserError(409))
		const session = createSession({
			createFolder,
			readText: vi.fn().mockResolvedValue('{"version":1,"items":[]}'),
		})
		const options = moveToRecycleBinMutationOptions(queryClient, session)

		await options.mutationFn?.({
			isDir: false,
			path: '/event-backup.20250907.full.tar.gz',
			size: 1,
		}, mutationContext)

		expect(createFolder).toHaveBeenCalledWith('/.storage-manager-trash')
		expect(createFolder).toHaveBeenCalledWith('/.storage-manager-trash/objects')
		expect(session.client.move).toHaveBeenCalledWith(
			'/event-backup.20250907.full.tar.gz',
			expect.stringMatching(/^\/\.storage-manager-trash\/objects\/.+-event-backup\.20250907\.full\.tar\.gz$/),
			true,
		)
	})

	it('updates text query data after saving files', async () => {
		const queryClient = new QueryClient()
		const session = createSession()
		const options = saveFileTextMutationOptions(queryClient, session)
		const input = { path: '/readme.md', content: '# updated' }

		const result = await options.mutationFn?.(input, mutationContext)
		await options.onSuccess?.(result!, input, undefined, mutationContext)

		expect(session.client.saveText).toHaveBeenCalledWith('/readme.md', '# updated')
		expect(queryClient.getQueryData(fileManagerKeys.text('pvc-1', '/readme.md'))).toBe('# updated')
	})

	it('tracks upload task progress and session identity', async () => {
		uploadActions.reset()
		const queryClient = new QueryClient()
		const file = new File(['hello'], 'hello.txt')
		const uploadFile = vi.fn(async (_path, _file, options) => {
			options.onProgress({ bytesUploaded: 5, bytesTotal: 5, chunkSize: 5 })
		})
		const session = createSession({ uploadFile })
		const options = uploadFileMutationOptions(queryClient, session)

		const result = await options.mutationFn?.({
			currentPath: '/',
			file,
			podSessionID: 'ps-1',
			viewerSessionID: 'vs-1',
		}, mutationContext)

		expect(result?.path).toBe('/hello.txt')
		expect(uploadStore.state.tasks[0]).toMatchObject({
			id: result?.taskID,
			fileName: 'hello.txt',
			podSessionID: 'ps-1',
			pvcKey: 'pvc-1',
			status: 'success',
			viewerSessionID: 'vs-1',
		})
	})

	it('keeps intra-chunk upload progress out of the store until a chunk is accepted', async () => {
		uploadActions.reset()
		const queryClient = new QueryClient()
		const file = new File(['hello world'], 'hello.txt')
		const updateSpy = vi.spyOn(uploadActions, 'updateTask')
		const uploadFile = vi.fn(async (_path, _file, options) => {
			options.onProgress({ bytesUploaded: 1, bytesTotal: 11 })
			options.onProgress({ bytesUploaded: 2, bytesTotal: 11 })
			options.onProgress({ bytesUploaded: 8, bytesTotal: 11, chunkSize: 8 })
		})
		const session = createSession({ uploadFile })
		const options = uploadFileMutationOptions(queryClient, session)

		await options.mutationFn?.({
			currentPath: '/',
			file,
			taskID: 'task-dialog-1',
		}, mutationContext)

		expect(uploadStore.state.tasks[0]).toMatchObject({
			bytesUploaded: 11,
			chunkIndex: 1,
			status: 'success',
		})
		expect(uploadFile).toHaveBeenCalledWith('/', file, expect.objectContaining({
			onProgress: expect.any(Function),
		}))
		expect(updateSpy.mock.calls.map(call => call[1])).toEqual([
			expect.objectContaining({ bytesUploaded: 8, chunkIndex: 1 }),
			expect.objectContaining({ bytesUploaded: 11, chunkIndex: 1 }),
			expect.objectContaining({ bytesUploaded: 11, chunkIndex: 1, status: 'success' }),
		])
		updateSpy.mockRestore()
		uploadActions.updateTask = originalUpdateTask
	})

	it('uses a caller-provided upload task id for dialog-scoped progress', async () => {
		uploadActions.reset()
		const queryClient = new QueryClient()
		const file = new File(['hello'], 'hello.txt')
		const session = createSession()
		const options = uploadFileMutationOptions(queryClient, session)

		const result = await options.mutationFn?.({
			currentPath: '/',
			file,
			taskID: 'task-dialog-1',
		}, mutationContext)

		expect(result?.taskID).toBe('task-dialog-1')
		expect(uploadStore.state.tasks[0]?.id).toBe('task-dialog-1')
	})

	it('uploads folder files sequentially, creates nested directories, and tolerates existing directories', async () => {
		uploadActions.reset()
		const queryClient = new QueryClient()
		const calls: string[] = []
		const createFolder = vi.fn(async (path: string) => {
			calls.push(`mkdir:${path}`)
			if (path === '/docs/assets') {
				throw fileBrowserError(409)
			}
		})
		const uploadFile = vi.fn(async (path: string, file: File) => {
			calls.push(`upload:${path}/${file.name}`)
		})
		const session = createSession({ createFolder, uploadFile })
		const options = uploadFilesMutationOptions(queryClient, session)
		const result = await options.mutationFn?.({
			currentPath: '/docs',
			files: [
				{ file: new File(['a'], 'one.txt'), relativePath: 'assets/one.txt' },
				{ file: new File(['b'], 'two.txt'), relativePath: 'assets/nested/two.txt' },
			],
		}, mutationContext)

		expect(result).toMatchObject({ failed: 0, succeeded: 2, uploadedPaths: ['/docs/assets/one.txt', '/docs/assets/nested/two.txt'] })
		expect(createFolder.mock.calls.map(([path]) => path)).toEqual([
			'/docs/assets',
			'/docs/assets/nested',
		])
		expect(uploadFile.mock.calls.map(([path, file]) => `${path}/${(file as File).name}`)).toEqual([
			'/docs/assets/one.txt',
			'/docs/assets/nested/two.txt',
		])
		expect(calls).toEqual([
			'mkdir:/docs/assets',
			'upload:/docs/assets/one.txt',
			'mkdir:/docs/assets/nested',
			'upload:/docs/assets/nested/two.txt',
		])
		expect(uploadStore.state.tasks).toHaveLength(2)
		expect(uploadStore.state.tasks).toEqual(expect.arrayContaining([
			expect.objectContaining({ fileName: 'assets/one.txt', status: 'success', targetPath: '/docs/assets' }),
			expect.objectContaining({ fileName: 'assets/nested/two.txt', status: 'success', targetPath: '/docs/assets/nested' }),
		]))
	})

	it('keeps failed files isolated so later files continue uploading', async () => {
		uploadActions.reset()
		const queryClient = new QueryClient()
		const uploadFile = vi.fn(async (_path: string, file: File) => {
			if (file.name === 'bad.txt') {
				throw new Error('network down')
			}
		})
		const session = createSession({ uploadFile })
		const options = uploadFilesMutationOptions(queryClient, session)
		const result = await options.mutationFn?.({
			currentPath: '/',
			files: [
				{ file: new File(['bad'], 'bad.txt') },
				{ file: new File(['good'], 'good.txt') },
			],
		}, mutationContext)

		expect(result).toMatchObject({ failed: 1, succeeded: 1, uploadedPaths: ['/good.txt'] })
		expect(uploadFile).toHaveBeenCalledTimes(2)
		expect(uploadStore.state.tasks).toEqual(expect.arrayContaining([
			expect.objectContaining({ fileName: 'bad.txt', status: 'failed', errorMessage: 'network down' }),
			expect.objectContaining({ fileName: 'good.txt', status: 'success' }),
		]))
	})

	it('rejects traversal paths without touching the client', async () => {
		uploadActions.reset()
		const queryClient = new QueryClient()
		const createFolder = vi.fn().mockResolvedValue(undefined)
		const uploadFile = vi.fn().mockResolvedValue(undefined)
		const session = createSession({ createFolder, uploadFile })
		const options = uploadFilesMutationOptions(queryClient, session)
		const result = await options.mutationFn?.({
			currentPath: '/docs',
			files: [{ file: new File(['x'], 'x.txt'), relativePath: '../x.txt' }],
		}, mutationContext)

		expect(result?.failed).toBe(1)
		expect(result?.succeeded).toBe(0)
		expect(createFolder).not.toHaveBeenCalled()
		expect(uploadFile).not.toHaveBeenCalled()
		expect(uploadStore.state.tasks[0]).toMatchObject({ status: 'failed', errorMessage: 'Upload paths cannot contain parent-directory segments' })
	})

	it('throttles high-frequency upload progress within the same chunk', () => {
		const first = {
			bytesUploaded: 1,
			bytesTotal: 100,
			chunkIndex: 1,
			chunkTotal: 10,
		}
		const sameChunk = {
			bytesUploaded: 2,
			bytesTotal: 100,
			chunkIndex: 1,
			chunkTotal: 10,
		}
		const nextChunk = {
			bytesUploaded: 11,
			bytesTotal: 100,
			chunkIndex: 2,
			chunkTotal: 10,
		}

		expect(shouldReportUploadProgress({
			current: first,
			last: null,
			lastReportedAt: 0,
			now: 1_000,
		})).toBe(true)
		expect(shouldReportUploadProgress({
			current: sameChunk,
			last: first,
			lastReportedAt: 1_000,
			now: 1_100,
		})).toBe(false)
		expect(shouldReportUploadProgress({
			current: sameChunk,
			last: first,
			lastReportedAt: 1_000,
			now: 1_300,
		})).toBe(true)
		expect(shouldReportUploadProgress({
			current: nextChunk,
			last: first,
			lastReportedAt: 1_000,
			now: 1_100,
		})).toBe(true)
	})
})
