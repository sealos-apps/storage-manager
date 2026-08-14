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

	it('closes the upload dialog while uploading and tracks the viewer session identity', async () => {
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

		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
		expect(await screen.findByText('0 file(s) uploaded')).toBeInTheDocument()
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

	it('supports selecting and uploading multiple files in one batch', async () => {
		const user = userEvent.setup()
		const uploadFile = vi.fn(async () => undefined)
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [])),
			uploadFile,
		})

		renderFileManager(session)

		await screen.findByText(/current directory is empty/i)
		await user.click(screen.getByRole('button', { name: /upload file/i }))
		const input = document.querySelector('input[type="file"]') as HTMLInputElement
		await user.upload(input, [
			new File(['one'], 'one.txt'),
			new File(['two'], 'two.txt'),
		])

		const dialog = screen.getByRole('dialog')
		expect(within(dialog).getByText(/2 file\(s\) selected/i)).toBeInTheDocument()
		await user.click(within(dialog).getByRole('button', { name: /upload file/i }))

		await waitFor(() => expect(uploadFile).toHaveBeenCalledTimes(2))
		expect(uploadFile).toHaveBeenNthCalledWith(1, '/', expect.objectContaining({ name: 'one.txt' }), expect.any(Object))
		expect(uploadFile).toHaveBeenNthCalledWith(2, '/', expect.objectContaining({ name: 'two.txt' }), expect.any(Object))
	})

	it('keeps only failed files for retry and shows each failure reason', async () => {
		const user = userEvent.setup()
		const calls: string[] = []
		let badAttempts = 0
		const uploadFile = vi.fn(async (_path: string, file: File) => {
			calls.push(file.name)
			if (file.name === 'bad.txt' && badAttempts++ === 0) {
				throw new Error('network down')
			}
		})
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [])),
			uploadFile,
		})

		renderFileManager(session)

		await screen.findByText(/current directory is empty/i)
		await user.click(screen.getByRole('button', { name: /upload file/i }))
		const input = document.querySelector('input[type="file"]') as HTMLInputElement
		await user.upload(input, [
			new File(['good'], 'good.txt'),
			new File(['bad'], 'bad.txt'),
		])

		let dialog = screen.getByRole('dialog')
		await user.click(within(dialog).getByRole('button', { name: /upload file/i }))

		await waitFor(() => expect(uploadFile).toHaveBeenCalledTimes(2))
		dialog = screen.getByRole('dialog')
		expect(within(dialog).getByText(/1 file\(s\) selected/i)).toBeInTheDocument()
		expect(within(dialog).getByText('bad.txt')).toBeInTheDocument()
		expect(within(dialog).queryByText('good.txt')).not.toBeInTheDocument()
		expect(within(dialog).getByText('network down')).toBeInTheDocument()

		await user.click(within(dialog).getByRole('button', { name: /upload file/i }))
		await waitFor(() => expect(uploadFile).toHaveBeenCalledTimes(3))
		expect(calls).toEqual(['good.txt', 'bad.txt', 'bad.txt'])
		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	})

	it('keeps relative paths when selecting a folder', async () => {
		const user = userEvent.setup()
		const uploadFile = vi.fn(async () => undefined)
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [])),
			uploadFile,
			createFolder: vi.fn(async () => undefined),
		})

		renderFileManager(session)

		await screen.findByText(/current directory is empty/i)
		await user.click(screen.getByRole('button', { name: /upload file/i }))
		const folderInput = document.querySelectorAll('input[type="file"]')[1] as HTMLInputElement
		Object.defineProperty(folderInput, 'webkitdirectory', { configurable: true, value: true })
		const folderFile = new File(['contents'], 'report.csv') as File & { webkitRelativePath?: string }
		Object.defineProperty(folderFile, 'webkitRelativePath', { configurable: true, value: 'reports/2026/report.csv' })
		await user.upload(folderInput, folderFile)

		const dialog = screen.getByRole('dialog')
		expect(within(dialog).getByText('reports/2026/report.csv')).toBeInTheDocument()
		await user.click(within(dialog).getByRole('button', { name: /upload file/i }))

		await waitFor(() => expect(uploadFile).toHaveBeenCalledTimes(1))
		expect(uploadFile).toHaveBeenCalledWith('/reports/2026', expect.objectContaining({ name: 'report.csv' }), expect.any(Object))
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
		expect(screen.queryByText('large.bin')).not.toBeInTheDocument()
		expect(await screen.findByText('0 file(s) uploaded')).toBeInTheDocument()
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
