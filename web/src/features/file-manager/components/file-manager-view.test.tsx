import type { FileBrowserResource } from '@sealos-storage-manager/filebrowser-client'
import type { ComponentProps } from 'react'
import type { FileBrowserSession } from '@/features/file-manager/types/file-manager'

import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { FileManagerView } from '@/features/file-manager/components/file-manager-view'
import { uploadActions, uploadStore } from '@/features/file-manager/stores/upload-store'
import { createFakeViewerAPI, pvcFixture, viewerSessionFixture, viewerTokenFixture } from '@/features/viewer/test/fakes'
import { deriveSessionCapability } from '@/features/viewer/utils/session-capability'
import { renderWithProviders } from '@/test/render'

vi.mock('@monaco-editor/react', () => ({
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

function resource(path: string, name: string, isDir: boolean, items?: FileBrowserResource[]): FileBrowserResource {
	return {
		isDir,
		items,
		modified: '2026-05-14T10:00:00Z',
		name,
		path,
		size: isDir ? 0 : 12,
	}
}

function readyCapability() {
	return deriveSessionCapability({
		error: null,
		isReconnecting: false,
		manualCloseKind: null,
		selectedPVC: pvcFixture(),
		session: viewerSessionFixture({ status: 'ready', token_ready: true }),
		status: 'ready',
		token: viewerTokenFixture(),
	})
}

function reconnectingCapability() {
	return deriveSessionCapability({
		error: null,
		isReconnecting: true,
		manualCloseKind: null,
		selectedPVC: pvcFixture(),
		session: viewerSessionFixture({ status: 'ready', token_ready: true }),
		status: 'failed',
		token: null,
	})
}

interface RenderFileManagerOptions {
	api?: ComponentProps<typeof FileManagerView>['api']
	currentPath?: string
	onManualClose?: ComponentProps<typeof FileManagerView>['onManualClose']
	onPathChange?: (path: string) => void
	onRefreshSession?: () => void
	podSessionID?: string | null
	sessionCapability?: ComponentProps<typeof FileManagerView>['sessionCapability']
	viewerSession?: ComponentProps<typeof FileManagerView>['viewerSession']
	viewerSessionID?: string | null
}

function renderFileManager(
	session: FileBrowserSession | null,
	currentPathOrOptions: string | RenderFileManagerOptions = '/',
	onPathChange = vi.fn(),
) {
	const options: RenderFileManagerOptions = typeof currentPathOrOptions === 'string'
		? { currentPath: currentPathOrOptions, onPathChange }
		: currentPathOrOptions
	const capability = options.sessionCapability ?? (session
		? readyCapability()
		: deriveSessionCapability({
				error: null,
				isReconnecting: false,
				manualCloseKind: null,
				selectedPVC: pvcFixture(),
				session: viewerSessionFixture({ status: 'creating', token_ready: false }),
				status: 'polling',
				token: null,
			}))

	return renderWithProviders(
		<FileManagerView
			api={options.api}
			currentPath={options.currentPath ?? '/'}
			onBackToVolumes={vi.fn()}
			onManualClose={options.onManualClose}
			onPathChange={options.onPathChange ?? vi.fn()}
			onReconnect={vi.fn()}
			onRefreshSession={options.onRefreshSession ?? vi.fn()}
			podSessionID={options.podSessionID}
			pvcName="data"
			session={session}
			sessionCapability={capability}
			setSort={vi.fn()}
			sort={{ field: 'name', direction: 'asc' }}
			viewerSession={options.viewerSession}
			viewerSessionID={options.viewerSessionID}
		/>,
	)
}

function sessionWithClient(client: Record<string, unknown>): FileBrowserSession {
	return {
		client: {
			usage: vi.fn().mockResolvedValue({ total: 20 * 1024 * 1024 * 1024, used: 5 * 1024 * 1024 * 1024 }),
			...client,
		},
		pvcKey: 'pvc-1',
	} as unknown as FileBrowserSession
}

describe('fileManagerView', () => {
	beforeEach(() => {
		uploadActions.reset()
	})

	it('hides the file table when the viewer session is not ready', () => {
		renderFileManager(null)

		expect(screen.getByText(/pod session is available/i)).toBeInTheDocument()
		expect(screen.getByRole('button', { name: /session status/i })).toBeInTheDocument()
		expect(screen.queryByRole('columnheader', { name: /name/i })).not.toBeInTheDocument()
		expect(screen.queryByRole('button', { name: /new folder/i })).not.toBeInTheDocument()
	})

	it('shows session details from the file manager title status popover', async () => {
		const user = userEvent.setup()
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [
				resource('/readme.md', 'readme.md', false),
			])),
		})
		const viewerSession = viewerSessionFixture({
			id: 'vs_ready',
			mode: 'readwrite',
			pod_session_id: 'ps_ready',
			pod_status: 'ready',
			status: 'ready',
			token_ready: true,
			viewer_url: 'https://viewer.example.test',
		})

		renderFileManager(session, {
			podSessionID: 'ps_ready',
			viewerSession,
			viewerSessionID: 'vs_ready',
		})

		await screen.findByText('readme.md')
		await user.click(screen.getByRole('button', { name: /session status/i }))

		const popover = await screen.findByText('https://viewer.example.test')
		expect(popover).toBeInTheDocument()
		expect(screen.getByText('ps_ready')).toBeInTheDocument()
		expect(screen.getByText('readwrite')).toBeInTheDocument()
		expect(screen.queryByRole('button', { name: /^retry$/i })).not.toBeInTheDocument()
		expect(screen.getByRole('button', { name: /close viewer/i })).toBeInTheDocument()
	})

	it('expands folder rows and renders all returned children without a page limit', async () => {
		const user = userEvent.setup()
		const childItems = Array.from({ length: 30 }, (_, index) =>
			resource(`/docs/file-${index}.txt`, `file-${index}.txt`, false))
		const list = vi.fn(async (path: string) => {
			if (path === '/docs') {
				return resource('/docs', 'docs', true, childItems)
			}
			return resource('/', '', true, [
				resource('/docs', 'docs', true),
				resource('/readme.md', 'readme.md', false),
			])
		})
		const session = sessionWithClient({ list })

		renderFileManager(session)

		await screen.findByText('docs')
		await user.click(screen.getAllByRole('button', { name: /toggle folder/i })[0]!)

		await waitFor(() => expect(list).toHaveBeenCalledWith('/docs', expect.any(AbortSignal)))
		expect(await screen.findByText('file-29.txt')).toBeInTheDocument()
		expect(screen.getByText('file-0.txt')).toBeInTheDocument()
		expect(screen.queryByText(/enter folder/i)).not.toBeInTheDocument()
	})

	it('shows branch loading in the folder chevron without adding a placeholder row', async () => {
		const user = userEvent.setup()
		let resolveDocs: (value: FileBrowserResource) => void = () => undefined
		const docsPromise = new Promise<FileBrowserResource>((resolve) => {
			resolveDocs = resolve
		})
		const list = vi.fn(async (path: string) => {
			if (path === '/docs') {
				return docsPromise
			}
			return resource('/', '', true, [
				resource('/docs', 'docs', true),
				resource('/readme.md', 'readme.md', false),
			])
		})
		const session = sessionWithClient({ list })

		renderFileManager(session)

		await screen.findByText('docs')
		await user.click(screen.getByRole('button', { name: /toggle folder/i }))

		expect(screen.getByText('docs')).toBeInTheDocument()
		expect(screen.getByText('readme.md')).toBeInTheDocument()
		expect(screen.queryByText(/pending file list/i)).not.toBeInTheDocument()

		resolveDocs(resource('/docs', 'docs', true, []))
		await waitFor(() => expect(list).toHaveBeenCalledWith('/docs', expect.any(AbortSignal)))
	})

	it('opens folders from the entry name', async () => {
		const user = userEvent.setup()
		const onPathChange = vi.fn()
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [
				resource('/docs', 'docs', true),
			])),
		})

		renderFileManager(session, '/', onPathChange)

		await user.click(await screen.findByText('docs'))

		expect(onPathChange).toHaveBeenCalledWith('/docs')
	})

	it('keeps file table columns fixed and formats modified time for reading', async () => {
		const modified = '2026-05-14T10:00:00Z'
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [
				{ ...resource('/readme.md', 'readme.md', false), modified },
			])),
		})

		renderFileManager(session)

		await screen.findByText('readme.md')
		const table = screen.getByRole('table')
		expect(table).toHaveClass('table-fixed')
		expect(table.querySelector('colgroup')).not.toBeNull()
		expect(table.querySelectorAll('col')).toHaveLength(4)
		const modifiedTime = table.querySelector(`time[datetime="${modified}"]`)
		expect(modifiedTime).not.toBeNull()
		expect(modifiedTime).not.toHaveTextContent(modified)
		expect(modifiedTime?.textContent).toContain('2026')
		expect(modifiedTime).toHaveAttribute('title', expect.stringContaining('2026'))
	})

	it('shows mounted storage usage from File Browser after the viewer is ready', async () => {
		const usage = vi.fn().mockResolvedValue({
			total: 20 * 1024 * 1024 * 1024,
			used: 5 * 1024 * 1024 * 1024,
		})
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [
				resource('/readme.md', 'readme.md', false),
			])),
			usage,
		})

		renderFileManager(session)

		expect(await screen.findByText('readme.md')).toBeInTheDocument()
		expect(await screen.findByText((_, element) => element?.textContent === '5 GiB / 20 GiB')).toBeInTheDocument()
		expect(screen.getByRole('progressbar', { name: /used/i }).querySelector('[data-slot="progress-indicator"]')).toHaveStyle({
			transform: 'translateX(-75%)',
		})
		expect(usage).toHaveBeenCalledWith('/', expect.any(AbortSignal))
	})

	it('keeps files usable when mounted storage usage cannot be read', async () => {
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [
				resource('/readme.md', 'readme.md', false),
			])),
			usage: vi.fn().mockRejectedValue(new Error('usage failed')),
		})

		renderFileManager(session)

		expect(await screen.findByText('readme.md')).toBeInTheDocument()
		expect(await screen.findByText(/capacity unavailable/i)).toBeInTheDocument()
		expect(screen.getByRole('table')).toBeInTheDocument()
	})

	it('refreshes mounted storage usage with the file list', async () => {
		const user = userEvent.setup()
		const list = vi.fn(async () => resource('/', '', true, [
			resource('/readme.md', 'readme.md', false),
		]))
		const usage = vi.fn().mockResolvedValue({ total: 100, used: 25 })
		const session = sessionWithClient({ list, usage })

		renderFileManager(session)

		await screen.findByText('readme.md')
		await waitFor(() => expect(usage).toHaveBeenCalledTimes(1))
		await user.click(screen.getByRole('button', { name: /refresh/i }))

		await waitFor(() => expect(list).toHaveBeenCalledTimes(2))
		await waitFor(() => expect(usage).toHaveBeenCalledTimes(2))
	})

	it('opens editable files from the entry name', async () => {
		const user = userEvent.setup()
		const readText = vi.fn().mockResolvedValue('hello')
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [
				resource('/readme.md', 'readme.md', false),
			])),
			readText,
		})

		renderFileManager(session)

		await user.click(await screen.findByText('readme.md'))

		expect(await screen.findByLabelText(/monaco editor/i)).toHaveValue('hello')
		expect(readText).toHaveBeenCalledWith('/readme.md', expect.any(AbortSignal))
	})

	it('keeps non-editable file names inert', async () => {
		const user = userEvent.setup()
		const downloadUrl = vi.fn(() => 'https://viewer.example.test/api/raw/archive.zip?auth=token')
		const readText = vi.fn()
		const session = sessionWithClient({
			downloadUrl,
			list: vi.fn(async () => resource('/', '', true, [
				resource('/archive.zip', 'archive.zip', false),
			])),
			readText,
		})

		renderFileManager(session)

		await user.click(await screen.findByText('archive.zip'))

		expect(downloadUrl).not.toHaveBeenCalled()
		expect(readText).not.toHaveBeenCalled()
		expect(screen.queryByLabelText(/monaco editor/i)).not.toBeInTheDocument()
	})

	it('does not freeze when an expanded folder response includes itself', async () => {
		const user = userEvent.setup()
		const list = vi.fn(async (path: string) => {
			if (path === '/docs') {
				return resource('/docs', 'docs', true, [
					resource('/docs', 'docs', true),
					resource('/docs/readme.md', 'readme.md', false),
				])
			}
			return resource('/', '', true, [
				resource('/docs', 'docs', true),
			])
		})
		const session = sessionWithClient({
			list,
		})

		renderFileManager(session)

		await screen.findByText('docs')
		await user.click(screen.getByRole('button', { name: /toggle folder/i }))

		expect(await screen.findByText('readme.md')).toBeInTheDocument()
		expect(screen.getAllByText('docs')).toHaveLength(1)
		expect(list).toHaveBeenCalledWith('/docs', expect.any(AbortSignal))
	})

	it('keeps previous rows visible with a pending overlay during a path change', async () => {
		let resolveDocs: (value: FileBrowserResource) => void = () => undefined
		const docsPromise = new Promise<FileBrowserResource>((resolve) => {
			resolveDocs = resolve
		})
		const list = vi.fn(async (path: string) => {
			if (path === '/docs') {
				return docsPromise
			}
			return resource('/', '', true, [
				resource('/readme.md', 'readme.md', false),
			])
		})
		const session = sessionWithClient({
			list,
		})
		const { rerender } = renderWithProviders(
			<FileManagerView
				currentPath="/"
				onBackToVolumes={vi.fn()}
				onPathChange={vi.fn()}
				onReconnect={vi.fn()}
				onRefreshSession={vi.fn()}
				pvcName="data"
				session={session}
				sessionCapability={readyCapability()}
				setSort={vi.fn()}
				sort={{ field: 'name', direction: 'asc' }}
			/>,
		)

		expect(await screen.findByText('readme.md')).toBeInTheDocument()
		rerender(
			<FileManagerView
				currentPath="/docs"
				onBackToVolumes={vi.fn()}
				onPathChange={vi.fn()}
				onReconnect={vi.fn()}
				onRefreshSession={vi.fn()}
				pvcName="data"
				session={session}
				sessionCapability={readyCapability()}
				setSort={vi.fn()}
				sort={{ field: 'name', direction: 'asc' }}
			/>,
		)

		expect(screen.getByText('readme.md')).toBeInTheDocument()
		expect(screen.getByRole('status')).toHaveTextContent(/pending file list/i)
		expect(screen.getByRole('button', { name: /download/i })).toBeDisabled()

		resolveDocs(resource('/docs', 'docs', true, [
			resource('/docs/next.txt', 'next.txt', false),
		]))
		expect(await screen.findByText('next.txt')).toBeInTheDocument()
		expect(screen.queryByRole('status')).not.toBeInTheDocument()
	})

	it('keeps the last file list visible and disabled while the viewer session reconnects', async () => {
		const list = vi.fn(async () => resource('/', '', true, [
			resource('/readme.md', 'readme.md', false),
		]))
		const session = sessionWithClient({
			list,
		})
		const { rerender } = renderWithProviders(
			<FileManagerView
				currentPath="/"
				onBackToVolumes={vi.fn()}
				onPathChange={vi.fn()}
				onReconnect={vi.fn()}
				onRefreshSession={vi.fn()}
				pvcName="data"
				session={session}
				sessionCapability={readyCapability()}
				setSort={vi.fn()}
				sort={{ field: 'name', direction: 'asc' }}
			/>,
		)

		expect(await screen.findByText('readme.md')).toBeInTheDocument()
		expect(list).toHaveBeenCalledTimes(1)

		rerender(
			<FileManagerView
				currentPath="/"
				onBackToVolumes={vi.fn()}
				onPathChange={vi.fn()}
				onReconnect={vi.fn()}
				onRefreshSession={vi.fn()}
				pvcName="data"
				session={session}
				sessionCapability={reconnectingCapability()}
				setSort={vi.fn()}
				sort={{ field: 'name', direction: 'asc' }}
			/>,
		)

		expect(screen.getByText('readme.md')).toBeInTheDocument()
		expect(screen.getByRole('status')).toHaveTextContent(/reconnecting viewer session/i)
		expect(screen.getByRole('button', { name: /download/i })).toBeDisabled()
		expect(list).toHaveBeenCalledTimes(1)
	})

	it('shows file list failure details with retry when the first list request fails', async () => {
		const user = userEvent.setup()
		const list = vi
			.fn()
			.mockRejectedValueOnce(new Error('list failed'))
			.mockResolvedValueOnce(resource('/', '', true, [
				resource('/recovered.txt', 'recovered.txt', false),
			]))
		const session = sessionWithClient({ list })

		renderFileManager(session, {
			podSessionID: 'ps_1',
			viewerSession: viewerSessionFixture({ id: 'vs_1', status: 'ready', token_ready: true }),
			viewerSessionID: 'vs_1',
		})

		expect(await screen.findByText(/file list unavailable/i)).toBeInTheDocument()
		expect(screen.getByText('list failed')).toBeInTheDocument()

		await user.click(screen.getByRole('button', { name: /^retry$/i }))

		expect(await screen.findByText('recovered.txt')).toBeInTheDocument()
		expect(list).toHaveBeenCalledTimes(2)
	})

	it('closes the viewer from the file list failure state', async () => {
		const user = userEvent.setup()
		const list = vi.fn().mockRejectedValue(new Error('list failed'))
		const closeViewerSession = vi.fn().mockResolvedValue(viewerSessionFixture({ id: 'vs_1', status: 'closed' }))
		const onManualClose = vi.fn()
		const api = createFakeViewerAPI({ closeViewerSession })
		const session = sessionWithClient({ list })

		renderFileManager(session, {
			api,
			onManualClose,
			podSessionID: 'ps_1',
			viewerSession: viewerSessionFixture({ id: 'vs_1', status: 'ready', token_ready: true }),
			viewerSessionID: 'vs_1',
		})

		expect(await screen.findByText(/file list unavailable/i)).toBeInTheDocument()
		await user.click(screen.getByRole('button', { name: /close viewer/i }))

		await waitFor(() => expect(closeViewerSession).toHaveBeenCalledWith('vs_1'))
		expect(onManualClose).toHaveBeenCalledWith('viewer')
	})

	it('shows a branch error row when folder expansion fails', async () => {
		const user = userEvent.setup()
		const list = vi.fn(async (path: string) => {
			if (path === '/docs') {
				throw new Error('folder failed')
			}
			return resource('/', '', true, [
				resource('/docs', 'docs', true),
			])
		})
		const session = sessionWithClient({
			list,
		})

		renderFileManager(session)

		await screen.findByText('docs')
		await user.click(screen.getByRole('button', { name: /toggle folder/i }))

		const row = await screen.findByText('folder failed')
		expect(within(row.closest('tr')!).getByRole('button', { name: /retry folder/i })).toBeInTheDocument()
	})

	it('creates folders through mutation options and refreshes the file tree cache', async () => {
		const user = userEvent.setup()
		const list = vi.fn(async () => resource('/', '', true, []))
		const createFolder = vi.fn().mockResolvedValue(undefined)
		const session = sessionWithClient({
			createFolder,
			list,
		})

		renderFileManager(session)

		await screen.findByText(/current directory is empty/i)
		await user.click(screen.getByRole('button', { name: /new folder/i }))
		await user.type(screen.getByLabelText(/folder name/i), 'test')
		await user.click(screen.getByRole('button', { name: /^create$/i }))

		await waitFor(() => expect(createFolder).toHaveBeenCalledWith('/test'))
		await waitFor(() => expect(list).toHaveBeenCalledTimes(2))
	})

	it('moves a directory named test to a raw trash destination path', async () => {
		const user = userEvent.setup()
		const move = vi.fn().mockResolvedValue(undefined)
		const session = sessionWithClient({
			createFolder: vi.fn().mockResolvedValue(undefined),
			list: vi.fn(async () => resource('/', '', true, [
				resource('/test', 'test', true),
			])),
			move,
			readText: vi.fn().mockResolvedValue('{"version":1,"items":[]}'),
			saveText: vi.fn().mockResolvedValue(undefined),
			writeText: vi.fn().mockResolvedValue(undefined),
		})

		renderFileManager(session)

		await screen.findByText('test')
		await user.click(screen.getByRole('button', { name: /delete/i }))
		await user.click(screen.getByRole('button', { name: /^delete$/i }))

		await waitFor(() => expect(move).toHaveBeenCalledWith(
			'/test',
			expect.stringMatching(/^\/\.storage-manager-trash\/objects\/.+-test$/),
			true,
		))
		expect(move.mock.calls[0]?.[1]).not.toContain('%2F')
	})

	it('loads editable text through a query and saves it through a mutation with a modal status', async () => {
		const user = userEvent.setup()
		let resolveSave: (() => void) | undefined
		const savePromise = new Promise<void>((resolve) => {
			resolveSave = resolve
		})
		const readText = vi.fn().mockResolvedValue('old content')
		const saveText = vi.fn().mockReturnValue(savePromise)
		const session = sessionWithClient({
			downloadUrl: vi.fn(() => 'https://viewer.example.test/api/raw/readme.md?auth=token'),
			list: vi.fn(async () => resource('/', '', true, [
				resource('/readme.md', 'readme.md', false),
			])),
			readText,
			saveText,
		})

		renderFileManager(session)

		await screen.findByText('readme.md')
		await user.click(screen.getByRole('button', { name: /edit/i }))
		expect(await screen.findByLabelText(/monaco editor/i)).toHaveValue('old content')
		expect(readText).toHaveBeenCalledWith('/readme.md', expect.any(AbortSignal))

		await user.clear(screen.getByLabelText(/monaco editor/i))
		await user.type(screen.getByLabelText(/monaco editor/i), 'new content')
		await user.click(screen.getByRole('button', { name: /^save$/i }))

		expect(await screen.findByRole('status')).toHaveTextContent(/saving file/i)
		expect(saveText).toHaveBeenCalledWith('/readme.md', 'new content')
		resolveSave?.()
		await waitFor(() => expect(screen.queryByText(/saving file/i)).not.toBeInTheDocument())
	})

	it('blocks browser editing for files larger than 32 MB', async () => {
		const user = userEvent.setup()
		const readText = vi.fn()
		const session = sessionWithClient({
			downloadUrl: vi.fn(() => 'https://viewer.example.test/api/raw/large.log?auth=token'),
			list: vi.fn(async () => resource('/', '', true, [
				{ ...resource('/large.log', 'large.log', false), size: 33 * 1024 * 1024 },
			])),
			readText,
		})

		renderFileManager(session)

		await screen.findByText('large.log')
		await user.click(screen.getByRole('button', { name: /edit/i }))

		expect(readText).not.toHaveBeenCalled()
		expect(screen.queryByLabelText(/monaco editor/i)).not.toBeInTheDocument()
	})

	it('uses browser-owned download URLs without fetching blobs in React', async () => {
		const user = userEvent.setup()
		const click = vi.fn()
		const originalCreateElement = document.createElement.bind(document)
		const createElementSpy = vi.spyOn(document, 'createElement').mockImplementation((tagName, options) => {
			const element = originalCreateElement(tagName, options)
			if (tagName.toLowerCase() === 'a') {
				Object.defineProperty(element, 'click', { value: click })
			}
			return element
		})
		const downloadBlob = vi.fn()
		const downloadUrl = vi.fn(() => 'https://viewer.example.test/api/raw/readme.md?auth=token')
		const session = sessionWithClient({
			downloadBlob,
			downloadUrl,
			list: vi.fn(async () => resource('/', '', true, [
				resource('/readme.md', 'readme.md', false),
			])),
		})

		try {
			renderFileManager(session)

			await screen.findByText('readme.md')
			await user.click(screen.getByRole('button', { name: /download/i }))

			expect(downloadUrl).toHaveBeenCalledWith('/readme.md')
			expect(downloadBlob).not.toHaveBeenCalled()
			expect(click).toHaveBeenCalled()
		}
		finally {
			createElementSpy.mockRestore()
		}
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
				onReconnect={vi.fn()}
				onRefreshSession={vi.fn()}
				podSessionID="ps-1"
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
