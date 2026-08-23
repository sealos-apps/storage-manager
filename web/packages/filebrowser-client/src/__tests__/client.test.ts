import { describe, expect, it, vi } from 'vitest'

import { FileBrowserClient } from '../client'
import { fileBrowserErrorCodeFromStatus } from '../errors'

describe('fileBrowserClient', () => {
	it('calls typed File Browser resource APIs with auth headers', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({
			path: '/',
			name: '',
			size: 0,
			modified: '',
			isDir: true,
			items: [],
		})))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test/',
			viewerSessionID: 'vs_1',
			authorization: 'Bearer user-auth',
			fetcher,
		})

		await expect(client.list('/docs')).resolves.toMatchObject({ path: '/' })

		expect(fetcher).toHaveBeenCalledWith('https://viewer.example.test/viewer-files/resources?viewer_session_id=vs_1&path=%2Fdocs', expect.objectContaining({
			method: 'GET',
			headers: expect.objectContaining({
				Authorization: 'Bearer user-auth',
			}),
		}))
		expect(fetcher.mock.calls[0]?.[1]?.headers).not.toHaveProperty('X-Auth')
	})

	it('reads File Browser disk usage for the mounted root', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({
			total: 20 * 1024 * 1024 * 1024,
			used: 7 * 1024 * 1024 * 1024,
		})))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test/',
			viewerSessionID: 'vs_1',
			authorization: 'Bearer user-auth',
			fetcher,
		})

		await expect(client.usage()).resolves.toEqual({
			total: 20 * 1024 * 1024 * 1024,
			used: 7 * 1024 * 1024 * 1024,
		})

		expect(fetcher).toHaveBeenCalledWith('https://viewer.example.test/viewer-files/usage?viewer_session_id=vs_1&path=%2F', expect.objectContaining({
			method: 'GET',
			headers: expect.objectContaining({
				Authorization: 'Bearer user-auth',
			}),
		}))
	})

	it('uses simple upload below the TUS threshold', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 200 }))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test',
			viewerSessionID: 'vs_1',
			authorization: 'Bearer user-auth',
			fetcher,
		})
		const onProgress = vi.fn()
		const file = new File(['small'], 'small.txt')

		await client.uploadFile('/', file, {
			thresholdBytes: 32 * 1024 * 1024,
			onProgress,
		})

		expect(fetcher).toHaveBeenCalledWith(
			'https://viewer.example.test/viewer-files/resources?viewer_session_id=vs_1&path=%2Fsmall.txt&override=false',
			expect.objectContaining({ method: 'POST', body: file }),
		)
		expect(onProgress).toHaveBeenCalledWith({
			bytesUploaded: file.size,
			bytesTotal: file.size,
		})
	})

	it('encodes simple upload file paths by segment', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 200 }))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test',
			viewerSessionID: 'vs_1',
			authorization: 'Bearer user-auth',
			fetcher,
		})
		const file = new File(['small'], '% done.txt')

		await client.uploadFile('/a folder/中文', file, {
			overwrite: true,
			thresholdBytes: 32 * 1024 * 1024,
		})

		const [url, init] = fetcher.mock.calls[0]!
		const parsed = new URL(url)
		expect(parsed.pathname).toBe('/viewer-files/resources')
		expect(parsed.searchParams.get('viewer_session_id')).toBe('vs_1')
		expect(parsed.searchParams.get('path')).toBe('/a folder/中文/% done.txt')
		expect(parsed.searchParams.get('override')).toBe('true')
		expect(init).toEqual(expect.objectContaining({ method: 'POST', body: file }))
	})

	it('downloads through the authenticated proxy without exposing a token in a URL', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response('file contents', { status: 200 }))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test/',
			viewerSessionID: 'vs_1',
			authorization: 'Bearer user-auth',
			fetcher,
		})

		await expect(client.downloadBlob('/a folder/test.txt')).resolves.toMatchObject({ size: 13 })

		const [url, init] = fetcher.mock.calls[0]!
		const parsed = new URL(url)
		expect(parsed.pathname).toBe('/viewer-files/raw')
		expect(parsed.searchParams.get('viewer_session_id')).toBe('vs_1')
		expect(parsed.searchParams.get('path')).toBe('/a folder/test.txt')
		expect(parsed.searchParams.get('auth')).toBeNull()
		expect(init).toEqual(expect.objectContaining({
			headers: { Authorization: 'Bearer user-auth' },
		}))
	})

	it('encodes source path segments and double-encodes destinations for File Browser move actions', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 200 }))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test',
			viewerSessionID: 'vs_1',
			authorization: 'Bearer user-auth',
			fetcher,
		})

		await client.move(
			'/a folder/中文/% done/test',
			'/.storage-manager-trash/objects/id-% done',
			true,
		)

		const [url, init] = fetcher.mock.calls[0]!
		expect(init).toEqual(expect.objectContaining({ method: 'PATCH' }))

		const parsed = new URL(url)
		expect(parsed.pathname).toBe('/viewer-files/resources')
		expect(parsed.searchParams.get('viewer_session_id')).toBe('vs_1')
		expect(parsed.searchParams.get('path')).toBe('/a folder/中文/% done/test')
		expect(parsed.searchParams.get('action')).toBe('rename')
		expect(parsed.searchParams.get('destination')).toBe('/.storage-manager-trash/objects/id-% done')
		expect(parsed.searchParams.get('override')).toBe('true')
		expect(parsed.searchParams.get('rename')).toBeNull()
	})

	it('normalizes conflict errors from File Browser responses', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({
			message: 'already exists',
		}), { status: 409 }))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test',
			viewerSessionID: 'vs_1',
			authorization: 'Bearer user-auth',
			fetcher,
		})

		await expect(client.createFolder('/docs')).rejects.toMatchObject({
			code: 'FILE_CONFLICT',
			status: 409,
		})
	})

	it('keeps a trailing slash when creating a folder', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 201 }))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test',
			viewerSessionID: 'vs_1',
			fetcher,
		})

		await client.createFolder('/docs')

		const [url] = fetcher.mock.calls[0]!
		expect(new URL(url).searchParams.get('path')).toBe('/docs/')
	})

	it('maps File Browser HTTP statuses to a closed error-code union', () => {
		expect(fileBrowserErrorCodeFromStatus(403)).toBe('FILEBROWSER_FORBIDDEN')
		expect(fileBrowserErrorCodeFromStatus(404)).toBe('FILEBROWSER_NOT_FOUND')
		expect(fileBrowserErrorCodeFromStatus(409)).toBe('FILE_CONFLICT')
		expect(fileBrowserErrorCodeFromStatus(599)).toBe('FILEBROWSER_REQUEST_FAILED')
	})
})
