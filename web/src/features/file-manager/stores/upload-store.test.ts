import { describe, expect, it } from 'vitest'

import { hasActiveUploadsForSession, summarizeUploadBatches, uploadActions, uploadStore } from '@/features/file-manager/stores/upload-store'

describe('uploadStore', () => {
	it('tracks upload task lifecycle in UI-only state', () => {
		uploadActions.reset()

		uploadActions.addTask({
			id: 'upload-1',
			fileName: 'large.bin',
			targetPath: '/',
			bytesUploaded: 0,
			bytesTotal: 100,
			pvcKey: 'pvc-1',
			viewerSessionID: 'vs-1',
			status: 'uploading',
		})
		expect(hasActiveUploadsForSession({ viewerSessionID: 'vs-1' })).toBe(true)
		uploadActions.updateTask('upload-1', {
			bytesUploaded: 100,
			status: 'success',
		})
		expect(hasActiveUploadsForSession({ viewerSessionID: 'vs-1' })).toBe(false)

		expect(uploadStore.state.tasks[0]).toMatchObject({
			bytesUploaded: 100,
			status: 'success',
		})

		uploadActions.clearCompleted()

		expect(uploadStore.state.tasks).toEqual([])
	})

	it('summarizes uploaded and failed files without counting tasks from another batch', () => {
		const summaries = summarizeUploadBatches([
			{
				batchID: 'batch-2',
				batchTotal: 2,
				bytesUploaded: 8,
				bytesTotal: 8,
				fileName: 'done.txt',
				id: 'upload-2',
				status: 'success',
				targetPath: '/',
			},
			{
				batchID: 'batch-2',
				batchTotal: 2,
				bytesUploaded: 0,
				bytesTotal: 8,
				fileName: 'failed.txt',
				id: 'upload-3',
				status: 'failed',
				targetPath: '/',
			},
			{
				batchID: 'batch-1',
				batchTotal: 1,
				bytesUploaded: 8,
				bytesTotal: 8,
				fileName: 'old.txt',
				id: 'upload-1',
				status: 'success',
				targetPath: '/',
			},
		])

		expect(summaries).toEqual([
			{ batchID: 'batch-2', failed: 1, isComplete: true, total: 2, uploaded: 1 },
			{ batchID: 'batch-1', failed: 0, isComplete: true, total: 1, uploaded: 1 },
		])
	})
})
