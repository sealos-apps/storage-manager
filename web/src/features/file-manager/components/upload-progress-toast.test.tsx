import { screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { UploadProgressToast } from '@/features/file-manager/components/upload-progress-toast'
import { uploadActions } from '@/features/file-manager/stores/upload-store'
import { renderWithProviders } from '@/test/render'

describe('uploadProgressToast', () => {
	beforeEach(() => {
		uploadActions.reset()
	})

	afterEach(() => {
		uploadActions.reset()
	})

	it('shows the uploaded count without exposing a denominator', async () => {
		renderWithProviders(<UploadProgressToast />)

		uploadActions.addTask({
			batchID: 'batch-1',
			batchTotal: 2,
			bytesUploaded: 0,
			bytesTotal: 8,
			fileName: 'one.txt',
			id: 'upload-1',
			status: 'uploading',
			targetPath: '/',
		})

		expect(await screen.findByText('0 file(s) uploaded')).toBeInTheDocument()
		uploadActions.updateTask('upload-1', { bytesUploaded: 8, status: 'success' })

		expect(await screen.findByText('1 file(s) uploaded')).toBeInTheDocument()
		expect(screen.queryByText(/of 2/i)).not.toBeInTheDocument()
	})

	it('shows failed files in the final batch summary', async () => {
		renderWithProviders(<UploadProgressToast />)

		uploadActions.addTask({
			batchID: 'batch-2',
			batchTotal: 2,
			bytesUploaded: 8,
			bytesTotal: 8,
			fileName: 'done.txt',
			id: 'upload-2',
			status: 'success',
			targetPath: '/',
		})
		uploadActions.addTask({
			batchID: 'batch-2',
			batchTotal: 2,
			bytesUploaded: 0,
			bytesTotal: 8,
			fileName: 'failed.txt',
			id: 'upload-3',
			status: 'failed',
			targetPath: '/',
		})

		expect(await screen.findByText('1 file(s) uploaded, 1 failed')).toBeInTheDocument()
	})

	it('mounts the toast at the bottom right', async () => {
		renderWithProviders(<UploadProgressToast />)
		uploadActions.addTask({
			batchID: 'batch-3',
			batchTotal: 1,
			bytesUploaded: 0,
			bytesTotal: 8,
			fileName: 'pending.txt',
			id: 'upload-4',
			status: 'uploading',
			targetPath: '/',
		})

		expect(await screen.findByText('0 file(s) uploaded')).toBeInTheDocument()
		expect(document.querySelector('[data-sonner-toaster][data-x-position="right"][data-y-position="bottom"]')).not.toBeNull()
	})
})
