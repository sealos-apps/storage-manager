import * as tus from 'tus-js-client'

import { FileBrowserError } from './errors'

export const defaultTusThresholdBytes = 32 * 1024 * 1024
export const defaultTusChunkBytes = 8 * 1024 * 1024

export interface UploadProgress {
	readonly bytesUploaded: number
	readonly bytesTotal: number
	readonly chunkSize?: number
}

export interface UploadOptions {
	readonly overwrite?: boolean
	readonly thresholdBytes?: number
	readonly chunkSizeBytes?: number
	readonly retryCount?: number
	readonly signal?: AbortSignal
	readonly onProgress?: (progress: UploadProgress) => void
}

export interface TusUploadOptions extends UploadOptions {
	readonly endpoint: string
	readonly fetcher?: typeof fetch
	readonly headers?: Readonly<Record<string, string>>
	readonly file: Blob
	readonly path: string
}

export function shouldUseTus(file: Blob, thresholdBytes = defaultTusThresholdBytes): boolean {
	return file.size >= thresholdBytes
}

export function retryDelays(retryCount = 5): number[] {
	if (retryCount <= 0) {
		return []
	}
	const delays: number[] = []
	let delay = 0
	for (let index = 0; index < retryCount; index += 1) {
		delays.push(Math.min(delay, 20_000))
		delay = delay === 0 ? 1_000 : Math.min(delay * 2, 20_000)
	}
	return delays
}

export function uploadTus(options: TusUploadOptions): Promise<void> {
	const uploadUrl = options.endpoint
	const fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis)
	return new Promise((resolve, reject) => {
		let settled = false
		const rejectOnce = (error: Error) => {
			if (settled) {
				return
			}
			settled = true
			reject(error)
		}
		const resolveOnce = () => {
			if (settled) {
				return
			}
			settled = true
			resolve()
		}

		const createUpload = async () => {
			const response = await fetcher(uploadUrl, {
				method: 'POST',
				headers: options.headers,
				signal: options.signal,
			})
			if (response.status !== 201) {
				throw new FileBrowserError({
					status: response.status,
					code: response.status === 409 ? 'FILE_CONFLICT' : 'TUS_UPLOAD_FAILED',
					message: await response.text().catch(() => response.statusText) || response.statusText || 'Failed to create TUS upload',
				})
			}
		}

		const upload = new tus.Upload(options.file, {
			uploadUrl,
			chunkSize: options.chunkSizeBytes ?? defaultTusChunkBytes,
			retryDelays: retryDelays(options.retryCount ?? 5),
			parallelUploads: 1,
			storeFingerprintForResuming: false,
			headers: options.headers,
			onShouldRetry(error) {
				const status = error.originalResponse?.getStatus() ?? 0
				return status !== 409
			},
			onError(error) {
				const status = error instanceof tus.DetailedError
					? error.originalResponse?.getStatus() ?? 0
					: 0
				rejectOnce(new FileBrowserError({
					status,
					code: status === 409 ? 'FILE_CONFLICT' : 'TUS_UPLOAD_FAILED',
					message: error.message,
				}))
			},
			onChunkComplete(chunkSize, bytesUploaded, bytesTotal) {
				options.onProgress?.({ bytesUploaded, bytesTotal, chunkSize })
			},
			onSuccess() {
				resolveOnce()
			},
		})
		if (options.signal) {
			if (options.signal.aborted) {
				upload.abort(true)
				rejectOnce(new DOMException('Upload aborted', 'AbortError'))
				return
			}
			options.signal.addEventListener('abort', () => {
				upload.abort(true)
				rejectOnce(new DOMException('Upload aborted', 'AbortError'))
			}, { once: true })
		}
		void createUpload()
			.then(() => {
				if (!settled) {
					upload.start()
				}
			})
			.catch((error: unknown) => {
				rejectOnce(error instanceof Error
					? error
					: new FileBrowserError({
							status: 0,
							code: 'TUS_UPLOAD_FAILED',
							message: 'Failed to create TUS upload',
						}))
			})
	})
}
