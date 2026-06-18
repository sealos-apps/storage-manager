import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { uploadActions } from '@/features/file-manager/stores/upload-store'
import { renderFileManager, resource, sessionWithClient } from '@/features/file-manager/test/file-manager-view-helpers'

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

describe('fileManagerActions', () => {
	beforeEach(() => {
		uploadActions.reset()
	})

	afterEach(() => {
		vi.useRealTimers()
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
})
