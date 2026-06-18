import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { FileManagerView } from '@/features/file-manager/components/file-manager-view'
import { uploadActions, uploadStore } from '@/features/file-manager/stores/upload-store'
import { readyCapability, renderFileManager, resource, sessionWithClient } from '@/features/file-manager/test/file-manager-view-helpers'
import { pvcFixture } from '@/features/viewer/test/fakes'
import { renderWithProviders } from '@/test/render'

vi.mock('@/components/monaco-editor', () => ({
	default: ({
		onChange,
		value,
	}: {
		onChange?: (value?: string) => void
		value?: string
	}) => (
		<textarea
			aria-label="Monaco editor"
			onChange={event => onChange?.(event.target.value)}
			value={value ?? ''}
		/>
	),
}))

describe('fileManagerUpload', () => {
	beforeEach(() => {
		uploadActions.reset()
	})

	afterEach(() => {
		vi.useRealTimers()
	})

	it('shows upload progress inside the dialog and tracks the viewer session identity', async () => {
		const user = userEvent.setup()
		let resolveUpload: (() => void) | undefined
		const uploadPromise = new Promise<void>((resolve) => {
			resolveUpload = resolve
		})
		const uploadFile = vi.fn(async (_path, _file, options) => {
			options.onProgress({ bytesUploaded: 4, bytesTotal: 8 })
			await uploadPromise
		})
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [])),
			uploadFile,
		})

		renderWithProviders(
			<FileManagerView
				currentPath="/"
				onBackToVolumes={vi.fn()}
				onPathChange={vi.fn()}
				onRefreshSession={vi.fn()}
				onRefreshStorageData={vi.fn()}
				podSessionID="ps-1"
				pvc={pvcFixture()}
				pvcName="data"
				session={session}
				sessionCapability={readyCapability()}
				setSort={vi.fn()}
				sort={{ field: 'name', direction: 'asc' }}
				viewerSessionID="vs-1"
			/>,
		)

		await screen.findByText(/current directory is empty/i)
		await user.click(screen.getByRole('button', { name: /upload file/i }))
		const input = document.querySelector('input[type="file"]') as HTMLInputElement
		await user.upload(input, new File(['contents'], 'demo.txt'))
		await user.click(screen.getAllByRole('button', { name: /upload file/i }).at(-1)!)

		expect(await screen.findByRole('status')).toHaveTextContent(/uploading file/i)
		resolveUpload?.()
		await waitFor(() => expect(uploadStore.state.tasks[0]).toMatchObject({
			fileName: 'demo.txt',
			podSessionID: 'ps-1',
			pvcKey: 'pvc-1',
			status: 'success',
			viewerSessionID: 'vs-1',
		}))
	})

	it('uploads files to the selected dialog target path', async () => {
		const user = userEvent.setup()
		const uploadFile = vi.fn(async () => undefined)
		const list = vi.fn(async (path: string) => {
			if (path === '/docs') {
				return resource('/docs', 'docs', true, [
					resource('/docs/nested', 'nested', true),
				])
			}
			return resource('/', '', true, [
				resource('/docs', 'docs', true),
				resource('/readme.md', 'readme.md', false),
			])
		})
		const session = sessionWithClient({
			list,
			uploadFile,
		})

		renderFileManager(session)

		await screen.findByText('readme.md')
		await user.click(screen.getByRole('button', { name: /upload file/i }))
		await user.click(await screen.findByRole('button', { name: 'docs' }))
		const input = document.querySelector('input[type="file"]') as HTMLInputElement
		await user.upload(input, new File(['contents'], 'demo.txt'))
		await user.click(screen.getAllByRole('button', { name: /upload file/i }).at(-1)!)

		await waitFor(() => expect(uploadFile).toHaveBeenCalled())
		expect(uploadFile).toHaveBeenCalledWith('/docs', expect.any(File), expect.any(Object))
	})

	it('keeps upload task progress updates out of the file table render path', async () => {
		const list = vi.fn(async () => resource('/', '', true, [
			resource('/readme.md', 'readme.md', false),
		]))
		const session = sessionWithClient({
			list,
		})

		renderFileManager(session)

		await screen.findByText('readme.md')
		expect(list).toHaveBeenCalledTimes(1)

		uploadActions.addTask({
			id: 'upload-1',
			fileName: 'large.bin',
			targetPath: '/',
			bytesUploaded: 0,
			bytesTotal: 100,
			status: 'uploading',
		})
		uploadActions.updateTask('upload-1', {
			bytesUploaded: 50,
		})

		expect(screen.getByText('readme.md')).toBeInTheDocument()
		expect(await screen.findByText('large.bin')).toBeInTheDocument()
		expect(list).toHaveBeenCalledTimes(1)
	})

	it('keeps failed upload state scoped to the current upload dialog attempt', async () => {
		const user = userEvent.setup()
		const uploadFile = vi.fn(async (_path, _file, options) => {
			options.onProgress({ bytesUploaded: 0, bytesTotal: 8 })
			throw new Error('chunk failed')
		})
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [])),
			uploadFile,
		})

		renderFileManager(session)

		await screen.findByText(/current directory is empty/i)
		await user.click(screen.getByRole('button', { name: /upload file/i }))
		let input = document.querySelector('input[type="file"]') as HTMLInputElement
		await user.upload(input, new File(['contents'], 'demo.txt'))
		await user.click(screen.getAllByRole('button', { name: /upload file/i }).at(-1)!)

		await waitFor(() => expect(screen.getAllByText('chunk failed').length).toBeGreaterThanOrEqual(1))
		await user.click(screen.getByRole('button', { name: /^cancel$/i }))
		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

		await user.click(screen.getByRole('button', { name: /upload file/i }))
		input = document.querySelector('input[type="file"]') as HTMLInputElement
		expect(input.value).toBe('')
		const dialog = await screen.findByRole('dialog')
		expect(within(dialog).queryByText('demo.txt')).not.toBeInTheDocument()
		expect(within(dialog).queryByText('chunk failed')).not.toBeInTheDocument()
	})
})
