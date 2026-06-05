import type { UploadOptions } from './upload'
import { errorFromResponse } from './errors'
import { encodePath, joinPath } from './path'
import { shouldUseTus, uploadTus } from './upload'

export interface FileBrowserClientOptions {
	readonly baseUrl: string
	readonly token: string
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
	private readonly token: string
	private readonly fetcher: typeof fetch

	constructor(options: FileBrowserClientOptions) {
		this.baseUrl = options.baseUrl.replace(/\/$/, '')
		this.token = options.token
		this.fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis)
	}

	async list(path = '/', signal?: AbortSignal): Promise<FileBrowserResource> {
		return this.json<FileBrowserResource>('GET', `/api/resources${encodePath(path)}`, { signal })
	}

	async listRecursive(path = '/', signal?: AbortSignal): Promise<RecursiveEntry[]> {
		return this.json<RecursiveEntry[]>('GET', `/api/resources/recursive${encodePath(path)}`, { signal })
	}

	async usage(path = '/', signal?: AbortSignal): Promise<FileBrowserUsage> {
		return this.json<FileBrowserUsage>('GET', `/api/usage${encodePath(path)}`, { signal })
	}

	downloadUrl(path: string, inline = false): string {
		const query = new URLSearchParams({
			auth: this.token,
		})
		if (inline) {
			query.set('inline', 'true')
		}
		return `${this.baseUrl}/api/raw${encodePath(path)}?${query.toString()}`
	}

	async createFolder(path: string): Promise<void> {
		const folderPath = encodePath(path).replace(/\/?$/, '/')
		await this.request('POST', `/api/resources${folderPath}`)
	}

	async uploadFile(parent: string, file: Blob & { name?: string }, options: UploadOptions = {}): Promise<void> {
		const filePath = joinPath(parent, file.name ?? 'upload.bin')
		if (shouldUseTus(file, options.thresholdBytes)) {
			await uploadTus({
				...options,
				endpoint: this.baseUrl,
				fetcher: this.fetcher,
				file,
				path: filePath,
				token: this.token,
			})
			return
		}
		await this.request('POST', `/api/resources${encodePath(filePath)}?override=${options.overwrite === true}`, {
			body: file,
			signal: options.signal,
		})
		options.onProgress?.({ bytesUploaded: file.size, bytesTotal: file.size })
	}

	async readText(path: string, signal?: AbortSignal): Promise<string> {
		const response = await this.request('GET', `/api/raw${encodePath(path)}?inline=true`, { signal })
		return response.text()
	}

	async downloadBlob(path: string, signal?: AbortSignal): Promise<Blob> {
		const response = await this.request('GET', `/api/raw${encodePath(path)}`, { signal })
		return response.blob()
	}

	async saveText(path: string, content: string): Promise<void> {
		await this.request('PUT', `/api/resources${encodePath(path)}`, { body: content })
	}

	async writeText(path: string, content: string, overwrite = true): Promise<void> {
		await this.request('POST', `/api/resources${encodePath(path)}?override=${overwrite}`, { body: content })
	}

	async move(source: string, destination: string, overwrite = false): Promise<void> {
		await this.patchAction('rename', source, destination, overwrite)
	}

	async copy(source: string, destination: string, overwrite = false): Promise<void> {
		await this.patchAction('copy', source, destination, overwrite)
	}

	async deletePermanent(path: string): Promise<void> {
		await this.request('DELETE', `/api/resources${encodePath(path)}`)
	}

	private async patchAction(action: 'rename' | 'copy', source: string, destination: string, overwrite: boolean): Promise<void> {
		const destinationParam = encodeURIComponent(encodePath(destination))
		const query = `action=${action}&destination=${destinationParam}&override=${overwrite}&rename=false`
		await this.request('PATCH', `/api/resources${encodePath(source)}?${query}`)
	}

	private async json<T>(method: string, path: string, init: RequestInit = {}): Promise<T> {
		const response = await this.request(method, path, init)
		return response.json() as Promise<T>
	}

	private async request(method: string, path: string, init: RequestInit = {}): Promise<Response> {
		const response = await this.fetcher(`${this.baseUrl}${path}`, {
			...init,
			method,
			headers: {
				'Authorization': `Bearer ${this.token}`,
				'X-Auth': this.token,
				...init.headers,
			},
		})
		if (!response.ok) {
			throw await errorFromResponse(response)
		}
		return response
	}
}
