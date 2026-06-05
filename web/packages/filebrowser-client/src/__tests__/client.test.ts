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
			token: 'token',
			fetcher,
		})

		await expect(client.list('/docs')).resolves.toMatchObject({ path: '/' })

		expect(fetcher).toHaveBeenCalledWith('https://viewer.example.test/api/resources/docs', expect.objectContaining({
			method: 'GET',
			headers: expect.objectContaining({
				'Authorization': 'Bearer token',
				'X-Auth': 'token',
			}),
		}))
	})

	it('reads File Browser disk usage for the mounted root', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({
			total: 20 * 1024 * 1024 * 1024,
			used: 7 * 1024 * 1024 * 1024,
		})))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test/',
			token: 'token',
			fetcher,
		})

		await expect(client.usage()).resolves.toEqual({
			total: 20 * 1024 * 1024 * 1024,
			used: 7 * 1024 * 1024 * 1024,
		})

		expect(fetcher).toHaveBeenCalledWith('https://viewer.example.test/api/usage/', expect.objectContaining({
			method: 'GET',
			headers: expect.objectContaining({
				'Authorization': 'Bearer token',
				'X-Auth': 'token',
			}),
		}))
	})

	it('uses simple upload below the TUS threshold', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 200 }))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test',
			token: 'token',
			fetcher,
		})
		const onProgress = vi.fn()
		const file = new File(['small'], 'small.txt')

		await client.uploadFile('/', file, {
			thresholdBytes: 32 * 1024 * 1024,
			onProgress,
		})

		expect(fetcher).toHaveBeenCalledWith(
			'https://viewer.example.test/api/resources/small.txt?override=false',
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
			token: 'token',
			fetcher,
		})
		const file = new File(['small'], '% done.txt')

		await client.uploadFile('/a folder/中文', file, {
			overwrite: true,
			thresholdBytes: 32 * 1024 * 1024,
		})

		expect(fetcher).toHaveBeenCalledWith(
			'https://viewer.example.test/api/resources/a%20folder/%E4%B8%AD%E6%96%87/%25%20done.txt?override=true',
			expect.objectContaining({ method: 'POST', body: file }),
		)
	})

	it('builds browser-owned download URLs with query auth', () => {
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test/',
			token: 'token with spaces',
			fetcher: vi.fn(),
		})

		const url = new URL(client.downloadUrl('/a folder/test.txt'))

		expect(url.pathname).toBe('/api/raw/a%20folder/test.txt')
		expect(url.searchParams.get('auth')).toBe('token with spaces')
		expect(url.searchParams.get('inline')).toBeNull()
	})

	it('encodes source path segments and double-encodes destinations for File Browser move actions', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 200 }))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test',
			token: 'token',
			fetcher,
		})

		await client.move(
			'/a folder/中文/% done/test',
			'/.storage-manager-trash/objects/id-% done',
			true,
		)

		const [url, init] = fetcher.mock.calls[0]!
		expect(init).toEqual(expect.objectContaining({ method: 'PATCH' }))
		expect(url).toContain('/api/resources/a%20folder/%E4%B8%AD%E6%96%87/%25%20done/test?')
		expect(url).toContain('destination=%2F.storage-manager-trash%2Fobjects%2Fid-%2525%2520done')

		const parsed = new URL(url)
		expect(parsed.searchParams.get('action')).toBe('rename')
		expect(parsed.searchParams.get('destination')).toBe('/.storage-manager-trash/objects/id-%25%20done')
		expect(decodeURIComponent(parsed.searchParams.get('destination') ?? '')).toBe('/.storage-manager-trash/objects/id-% done')
		expect(parsed.searchParams.get('override')).toBe('true')
		expect(parsed.searchParams.get('rename')).toBe('false')
	})

	it('normalizes conflict errors from File Browser responses', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({
			message: 'already exists',
		}), { status: 409 }))
		const client = new FileBrowserClient({
			baseUrl: 'https://viewer.example.test',
			token: 'token',
			fetcher,
		})

		await expect(client.createFolder('/docs')).rejects.toMatchObject({
			code: 'FILE_CONFLICT',
			status: 409,
		})
	})

	it('maps File Browser HTTP statuses to a closed error-code union', () => {
		expect(fileBrowserErrorCodeFromStatus(403)).toBe('FILEBROWSER_FORBIDDEN')
		expect(fileBrowserErrorCodeFromStatus(404)).toBe('FILEBROWSER_NOT_FOUND')
		expect(fileBrowserErrorCodeFromStatus(409)).toBe('FILE_CONFLICT')
		expect(fileBrowserErrorCodeFromStatus(599)).toBe('FILEBROWSER_REQUEST_FAILED')
	})
})
