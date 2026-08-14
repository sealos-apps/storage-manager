import type { QueryClient } from '@tanstack/react-query'
import type { UploadTask } from '@/features/file-manager/stores/upload-store'
import type { FileBrowserSession } from '@/features/file-manager/types/file-manager'

import { joinPath } from '@sealos-storage-manager/filebrowser-client'
import { mutationOptions } from '@tanstack/react-query'

import { env } from '@/config/env'
import { invalidateFileManagerAfterMutation } from '@/features/file-manager/api/file-manager-cache'
import { fileManagerKeys } from '@/features/file-manager/api/file-manager-query-keys'
import { requireSession } from '@/features/file-manager/api/file-manager-session'
import { uploadActions } from '@/features/file-manager/stores/upload-store'
import { resolveUploadPath as resolveSafeUploadPath, uploadRelativePathForFile } from '@/features/file-manager/utils/upload-path'

export interface UploadFileInput {
	currentPath: string
	file: File
	podSessionID?: string
	relativePath?: string
	taskID?: string
	viewerSessionID?: string
}

export interface UploadBatchFileInput {
	file: File
	relativePath?: string
}

export interface UploadFilesInput {
	currentPath: string
	files: UploadBatchFileInput[]
	podSessionID?: string
	viewerSessionID?: string
}

export interface UploadFileResult {
	errorMessage?: string
	fileName: string
	path: string
	status: 'failed' | 'success'
	taskID: string
}

export interface UploadFilesResult {
	createdDirectoryPaths: string[]
	failed: number
	results: UploadFileResult[]
	succeeded: number
	uploadedPaths: string[]
}

export interface UploadFileMutationResult {
	path: string
	taskID: string
}

export interface UploadProgressSnapshot {
	bytesTotal: number
	bytesUploaded: number
	chunkIndex: number
	chunkTotal: number
}

export function createUploadTaskID(fileName: string): string {
	const random = globalThis.crypto?.randomUUID?.() ?? Math.random().toString(36).slice(2, 10)
	return `${Date.now()}-${random}-${fileName}`
}

export function shouldReportUploadProgress(input: {
	current: UploadProgressSnapshot
	last: UploadProgressSnapshot | null
	lastReportedAt: number
	now: number
	throttleMs?: number
}): boolean {
	const throttleMs = input.throttleMs ?? 250
	if (!input.last) {
		return true
	}
	if (input.current.bytesUploaded >= input.current.bytesTotal) {
		return true
	}
	if (input.current.chunkIndex !== input.last.chunkIndex) {
		return true
	}
	return input.now - input.lastReportedAt >= throttleMs
}

function errorStatus(error: unknown): number | null {
	if (typeof error !== 'object' || error === null || !('status' in error)) {
		return null
	}
	return typeof error.status === 'number' ? error.status : null
}

interface ResolvedUploadPath {
	directoryPaths: string[]
	fileName: string
	parentPath: string
	path: string
	relativePath: string
}

function resolveUploadPath(currentPath: string, file: File, relativePath?: string): ResolvedUploadPath {
	const resolved = resolveSafeUploadPath(currentPath, uploadRelativePathForFile({
		name: file.name,
		relativePath,
	}))
	return {
		directoryPaths: resolved.directoryPaths,
		fileName: resolved.fileName,
		parentPath: resolved.parentPath,
		path: resolved.fullPath,
		relativePath: resolved.relativePath,
	}
}

function fileWithName(file: File, name: string): File {
	if (file.name === name) {
		return file
	}
	return new File([file], name, {
		lastModified: file.lastModified,
		type: file.type,
	})
}

async function uploadOne(
	input: UploadFileInput,
	activeSession: FileBrowserSession,
	throwOnError = false,
): Promise<UploadFileResult> {
	let resolved: ResolvedUploadPath
	try {
		resolved = resolveUploadPath(input.currentPath, input.file, input.relativePath)
	}
	catch (error) {
		const id = input.taskID ?? createUploadTaskID(input.relativePath || input.file.name)
		const errorMessage = error instanceof Error ? error.message : 'Upload failed'
		uploadActions.addTask({
			id,
			fileName: input.relativePath || input.file.name,
			targetPath: input.currentPath,
			bytesUploaded: 0,
			bytesTotal: input.file.size,
			podSessionID: input.podSessionID,
			pvcKey: activeSession.pvcKey,
			status: 'failed',
			viewerSessionID: input.viewerSessionID,
			errorMessage,
		})
		if (throwOnError) {
			throw error
		}
		return { errorMessage, fileName: input.relativePath || input.file.name, path: joinPath(input.currentPath, input.file.name), status: 'failed', taskID: id }
	}
	const id = input.taskID ?? createUploadTaskID(resolved.fileName)
	const chunkSizeBytes = env.fileUploadTusChunkBytes
	const chunkTotal = Math.max(1, Math.ceil(input.file.size / chunkSizeBytes))
	const task: UploadTask = {
		id,
		fileName: input.relativePath || resolved.fileName,
		targetPath: resolved.parentPath,
		bytesUploaded: 0,
		bytesTotal: input.file.size,
		chunkIndex: 0,
		chunkSizeBytes,
		chunkTotal,
		podSessionID: input.podSessionID,
		pvcKey: activeSession.pvcKey,
		status: 'uploading',
		viewerSessionID: input.viewerSessionID,
	}
	uploadActions.addTask(task)
	let lastProgress: UploadProgressSnapshot | null = null
	let lastReportedAt = 0
	try {
		const publishProgress = (snapshot: UploadProgressSnapshot, force = false) => {
			const now = Date.now()
			if (!force && !shouldReportUploadProgress({
				current: snapshot,
				last: lastProgress,
				lastReportedAt,
				now,
			})) {
				return
			}
			lastProgress = snapshot
			lastReportedAt = now
			uploadActions.updateTask(id, {
				bytesUploaded: snapshot.bytesUploaded,
				bytesTotal: snapshot.bytesTotal,
				chunkIndex: snapshot.chunkIndex,
				chunkTotal: snapshot.chunkTotal,
			})
		}
		await activeSession.client.uploadFile(resolved.parentPath, fileWithName(input.file, resolved.fileName), {
			chunkSizeBytes,
			retryCount: env.fileUploadTusRetryCount,
			thresholdBytes: env.fileUploadTusThresholdBytes,
			onProgress: (progress) => {
				if (!progress.chunkSize && progress.bytesUploaded < progress.bytesTotal) {
					return
				}
				publishProgress({
					bytesUploaded: progress.bytesUploaded,
					bytesTotal: progress.bytesTotal,
					chunkIndex: Math.min(
						chunkTotal,
						progress.chunkSize
							? Math.ceil(progress.bytesUploaded / chunkSizeBytes)
							: Math.floor(progress.bytesUploaded / chunkSizeBytes),
					),
					chunkTotal,
				})
			},
		})
		publishProgress({
			bytesUploaded: input.file.size,
			bytesTotal: input.file.size,
			chunkIndex: chunkTotal,
			chunkTotal,
		}, true)
		uploadActions.updateTask(id, {
			bytesUploaded: input.file.size,
			chunkIndex: chunkTotal,
			status: 'success',
		})
		return { fileName: input.relativePath || input.file.name, path: resolved.path, status: 'success', taskID: id }
	}
	catch (error) {
		const errorMessage = error instanceof Error ? error.message : 'Upload failed'
		uploadActions.updateTask(id, {
			errorMessage,
			status: 'failed',
		})
		if (throwOnError) {
			throw error
		}
		return { errorMessage, fileName: input.relativePath || input.file.name, path: resolved.path, status: 'failed', taskID: id }
	}
}

export function uploadFileMutationOptions(
	queryClient: QueryClient,
	session: FileBrowserSession | null,
) {
	return mutationOptions({
		mutationKey: fileManagerKeys.mutations.uploadFile(session?.pvcKey ?? 'inactive'),
		mutationFn: async (input: UploadFileInput) => {
			const activeSession = requireSession(session)
			const result = await uploadOne(input, activeSession, true)
			return { path: result.path, taskID: result.taskID } satisfies UploadFileMutationResult
		},
		onSuccess: ({ path }) => {
			if (!session) {
				return
			}
			invalidateFileManagerAfterMutation(queryClient, session, [path])
		},
	})
}

export function uploadFilesMutationOptions(
	queryClient: QueryClient,
	session: FileBrowserSession | null,
) {
	return mutationOptions({
		mutationKey: fileManagerKeys.mutations.uploadFile(session?.pvcKey ?? 'inactive'),
		mutationFn: async (input: UploadFilesInput): Promise<UploadFilesResult> => {
			const activeSession = requireSession(session)
			const results: UploadFileResult[] = []
			const createdDirectories = new Set<string>()
			for (const batchFile of input.files) {
				let resolved: ResolvedUploadPath | null = null
				try {
					resolved = resolveUploadPath(input.currentPath, batchFile.file, batchFile.relativePath)
					for (const directoryPath of resolved.directoryPaths) {
						if (createdDirectories.has(directoryPath)) {
							continue
						}
						try {
							await activeSession.client.createFolder(directoryPath)
						}
						catch (error) {
							if (errorStatus(error) !== 409) {
								throw error
							}
						}
						createdDirectories.add(directoryPath)
					}
				}
				catch (error) {
					const id = createUploadTaskID(batchFile.relativePath || batchFile.file.name)
					const errorMessage = error instanceof Error ? error.message : 'Upload failed'
					uploadActions.addTask({
						id,
						fileName: batchFile.relativePath || batchFile.file.name,
						targetPath: resolved?.parentPath ?? input.currentPath,
						bytesUploaded: 0,
						bytesTotal: batchFile.file.size,
						pvcKey: activeSession.pvcKey,
						podSessionID: input.podSessionID,
						status: 'failed',
						viewerSessionID: input.viewerSessionID,
						errorMessage,
					})
					results.push({ errorMessage, fileName: batchFile.relativePath || batchFile.file.name, path: resolved?.path ?? joinPath(input.currentPath, batchFile.file.name), status: 'failed', taskID: id })
					continue
				}
				results.push(await uploadOne({
					currentPath: input.currentPath,
					file: batchFile.file,
					podSessionID: input.podSessionID,
					relativePath: batchFile.relativePath,
					viewerSessionID: input.viewerSessionID,
				}, activeSession))
			}
			const uploadedPaths = results.filter(result => result.status === 'success').map(result => result.path)
			return {
				createdDirectoryPaths: [...createdDirectories],
				failed: results.filter(result => result.status === 'failed').length,
				results,
				succeeded: uploadedPaths.length,
				uploadedPaths,
			}
		},
		onSuccess: ({ createdDirectoryPaths, uploadedPaths }) => {
			const affectedPaths = [...new Set([...createdDirectoryPaths, ...uploadedPaths])]
			if (session && affectedPaths.length > 0) {
				invalidateFileManagerAfterMutation(queryClient, session, affectedPaths)
			}
		},
	})
}
