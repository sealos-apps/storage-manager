import type { FileBrowserResource } from '@sealos-storage-manager/filebrowser-client'

import { QueryClient } from '@tanstack/react-query'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { FileManagerView } from '@/features/file-manager/components/file-manager-view'
import { uploadActions } from '@/features/file-manager/stores/upload-store'
import { readyCapability, reconnectingCapability, renderFileManager, resource, sessionWithClient } from '@/features/file-manager/test/file-manager-view-helpers'
import { createFakeViewerAPI, viewerSessionFixture } from '@/features/viewer/test/fakes'
import { renderWithProviders } from '@/test/render'

describe('fileManagerView', () => {
	beforeEach(() => {
		uploadActions.reset()
	})

	afterEach(() => {
		vi.useRealTimers()
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
		vi.useFakeTimers({ shouldAdvanceTime: true })
		vi.setSystemTime(new Date('2026-05-14T10:00:30Z'))
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
		expect(modifiedTime).toHaveTextContent('30s ago')
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
		const usage = vi.fn().mockRejectedValue(new Error('usage failed'))
		const queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: 3,
				},
			},
		})
		const session = sessionWithClient({
			list: vi.fn(async () => resource('/', '', true, [
				resource('/readme.md', 'readme.md', false),
			])),
			usage,
		})

		renderFileManager(session, { queryClient })

		expect(await screen.findByText('readme.md')).toBeInTheDocument()
		expect(await screen.findByText(/capacity unavailable/i)).toBeInTheDocument()
		expect(screen.getByRole('table')).toBeInTheDocument()
		expect(usage).toHaveBeenCalledTimes(1)
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
		expect(list).toHaveBeenCalledTimes(1)

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
})
