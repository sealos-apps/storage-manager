import type { UploadOptions } from './upload'
import { errorFromResponse } from './errors'
import { joinPath, normalizePath } from './path'
import { shouldUseTus, uploadTus } from './upload'

export interface FileBrowserClientOptions {
	readonly baseUrl: string
	readonly viewerSessionID: string
	readonly authorization?: string
	readonly fetcher?: typeof fetch
}

export interface FileBrowserResource {
	readonly path: string
	readonly name: string
	readonly size: number
	readonly modified: string
	readonly isDir: boolean
	readonly type?: string
	readonly content?: string
	readonly items?: FileBrowserResource[]
}

export interface FileBrowserUsage {
	readonly total: number
	readonly used: number
}

export interface RecursiveEntry {
	readonly path: string
	readonly name: string
	readonly size: number
	readonly modified: string
	readonly isDir: boolean
}

export class FileBrowserClient {
	private readonly baseUrl: string
	private readonly viewerSessionID: string
	private readonly authorization?: string
	private readonly fetcher: typeof fetch

	constructor(options: FileBrowserClientOptions) {
		this.baseUrl = options.baseUrl.replace(/\/$/, '')
		this.viewerSessionID = options.viewerSessionID
		this.authorization = options.authorization
		this.fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis)
	}

	async list(path = '/', signal?: AbortSignal): Promise<FileBrowserResource> {
		return this.json<FileBrowserResource>('GET', 'resources', path, { signal })
	}

	async listRecursive(path = '/', signal?: AbortSignal): Promise<RecursiveEntry[]> {
		return this.json<RecursiveEntry[]>('GET', 'recursive', path, { signal })
	}

	async usage(path = '/', signal?: AbortSignal): Promise<FileBrowserUsage> {
		return this.json<FileBrowserUsage>('GET', 'usage', path, { signal })
	}

	async createFolder(path: string): Promise<void> {
		const normalizedPath = normalizePath(path)
		const folderPath = normalizedPath === '/' ? normalizedPath : `${normalizedPath}/`
		await this.request('POST', 'resources', folderPath)
	}

	async uploadFile(parent: string, file: Blob & { name?: string }, options: UploadOptions = {}): Promise<void> {
		const filePath = joinPath(parent, file.name ?? 'upload.bin')
		if (shouldUseTus(file, options.thresholdBytes)) {
			await uploadTus({
				...options,
				endpoint: this.gatewayURL('tus', filePath, { override: String(options.overwrite === true) }),
				fetcher: this.fetcher,
				file,
				path: filePath,
				headers: this.requestHeaders(),
			})
			return
		}
		await this.request('POST', 'resources', filePath, {
			body: file,
			signal: options.signal,
			query: { override: String(options.overwrite === true) },
		})
		options.onProgress?.({ bytesUploaded: file.size, bytesTotal: file.size })
	}

	async readText(path: string, signal?: AbortSignal): Promise<string> {
		const response = await this.request('GET', 'raw', path, { signal, query: { inline: 'true' } })
		return response.text()
	}

	async downloadBlob(path: string, signal?: AbortSignal): Promise<Blob> {
		const response = await this.request('GET', 'raw', path, { signal })
		return response.blob()
	}

	async saveText(path: string, content: string): Promise<void> {
		await this.request('PUT', 'resources', path, { body: content })
	}

	async writeText(path: string, content: string, overwrite = true): Promise<void> {
		await this.request('POST', 'resources', path, { body: content, query: { override: String(overwrite) } })
	}

	async move(source: string, destination: string, overwrite = false): Promise<void> {
		await this.patchAction('rename', source, destination, overwrite)
	}

	async copy(source: string, destination: string, overwrite = false): Promise<void> {
		await this.patchAction('copy', source, destination, overwrite)
	}

	async deletePermanent(path: string): Promise<void> {
		await this.request('DELETE', 'resources', path)
	}

	private async patchAction(action: 'rename' | 'copy', source: string, destination: string, overwrite: boolean): Promise<void> {
		await this.request('PATCH', 'resources', source, {
			query: {
				action,
				destination,
				override: String(overwrite),
			},
		})
	}

	private async json<T>(method: string, endpoint: string, path: string, init: FileBrowserRequestInit = {}): Promise<T> {
		const response = await this.request(method, endpoint, path, init)
		return response.json() as Promise<T>
	}

	private async request(method: string, endpoint: string, path: string, init: FileBrowserRequestInit = {}): Promise<Response> {
		const { query: _query, ...requestInit } = init
		const response = await this.fetcher(this.gatewayURL(endpoint, path, _query), {
			...requestInit,
			method,
			headers: { ...this.requestHeaders(), ...init.headers },
		})
		if (!response.ok) {
			throw await errorFromResponse(response)
		}
		return response
	}

	private gatewayURL(endpoint: string, path: string, query: Record<string, string> = {}) {
		const normalizedPath = normalizePath(path)
		const gatewayPath = normalizedPath !== '/' && path.trim().endsWith('/')
			? `${normalizedPath}/`
			: normalizedPath
		const params = new URLSearchParams({
			viewer_session_id: this.viewerSessionID,
			path: gatewayPath,
			...query,
		})
		return `${this.baseUrl}/viewer-files/${endpoint}?${params.toString()}`
	}

	private requestHeaders(): Record<string, string> {
		return this.authorization ? { Authorization: this.authorization } : {}
	}
}

interface FileBrowserRequestInit extends RequestInit {
	readonly query?: Record<string, string>
}
