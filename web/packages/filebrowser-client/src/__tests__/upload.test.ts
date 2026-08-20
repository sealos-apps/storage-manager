import { beforeEach, describe, expect, it, vi } from 'vitest'

import { retryDelays, shouldUseTus, uploadTus } from '../upload'

const tusState = vi.hoisted(() => ({
	options: undefined as Record<string, unknown> | undefined,
	abort: vi.fn(),
	start: vi.fn(),
}))

vi.mock('tus-js-client', () => {
	class DetailedError extends Error {
		originalResponse?: { getStatus: () => number }
	}

	class Upload {
		constructor(_file: Blob, options: Record<string, unknown>) {
			tusState.options = options
		}

		abort(shouldTerminate: boolean) {
			tusState.abort(shouldTerminate)
		}

		start() {
			tusState.start()
			const options = tusState.options as {
				onChunkComplete?: (chunkSize: number, bytesUploaded: number, bytesTotal: number) => void
				onProgress?: (bytesUploaded: number, bytesTotal: number) => void
				onSuccess?: () => void
			}
			options.onProgress?.(4, 8)
			options.onChunkComplete?.(4, 4, 8)
			options.onSuccess?.()
		}
	}

	return { DetailedError, Upload }
})

describe('tUS upload policy', () => {
	beforeEach(() => {
		tusState.options = undefined
		tusState.abort.mockClear()
		tusState.start.mockClear()
	})

	it('uses TUS at and above the large-file threshold', () => {
		expect(shouldUseTus(new Blob(['1234']), 4)).toBe(true)
		expect(shouldUseTus(new Blob(['123']), 4)).toBe(false)
	})

	it('creates exponential retry delays with an immediate first retry', () => {
		expect(retryDelays(5)).toEqual([0, 1000, 2000, 4000, 8000])
		expect(retryDelays(0)).toEqual([])
	})

	it('configures File Browser TUS uploads without persistent resume fingerprints', async () => {
		const onProgress = vi.fn()
		const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 201 }))

		await uploadTus({
			endpoint: 'https://viewer.example.test/viewer-files/tus?viewer_session_id=vs_1&path=%2Fdata.bin&override=true',
			fetcher,
			headers: { Authorization: 'Bearer user-auth' },
			file: new Blob(['payload']),
			path: '/data.bin',
			overwrite: true,
			chunkSizeBytes: 8,
			retryCount: 2,
			onProgress,
		})

		expect(tusState.start).toHaveBeenCalled()
		expect(fetcher).toHaveBeenCalledWith('https://viewer.example.test/viewer-files/tus?viewer_session_id=vs_1&path=%2Fdata.bin&override=true', expect.objectContaining({
			method: 'POST',
			headers: expect.objectContaining({
				Authorization: 'Bearer user-auth',
			}),
		}))
		expect(tusState.options).toMatchObject({
			uploadUrl: 'https://viewer.example.test/viewer-files/tus?viewer_session_id=vs_1&path=%2Fdata.bin&override=true',
			chunkSize: 8,
			retryDelays: [0, 1000],
			parallelUploads: 1,
			storeFingerprintForResuming: false,
			headers: {
				Authorization: 'Bearer user-auth',
			},
		})
		expect(onProgress).toHaveBeenCalledTimes(1)
		expect(onProgress).toHaveBeenCalledWith({ bytesUploaded: 4, bytesTotal: 8, chunkSize: 4 })
	})

	it('encodes File Browser TUS paths by segment before creating the upload', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 201 }))

		await uploadTus({
			endpoint: 'https://viewer.example.test/viewer-files/tus?viewer_session_id=vs_1&path=%2Fa+folder%2F%E4%B8%AD%E6%96%87%2F%25+done.bin&override=false',
			fetcher,
			headers: { Authorization: 'Bearer user-auth' },
			file: new Blob(['payload']),
			path: '/a folder/中文/% done.bin',
		})

		expect(fetcher).toHaveBeenCalledWith(
			'https://viewer.example.test/viewer-files/tus?viewer_session_id=vs_1&path=%2Fa+folder%2F%E4%B8%AD%E6%96%87%2F%25+done.bin&override=false',
			expect.objectContaining({ method: 'POST' }),
		)
		expect(tusState.options).toMatchObject({
			uploadUrl: 'https://viewer.example.test/viewer-files/tus?viewer_session_id=vs_1&path=%2Fa+folder%2F%E4%B8%AD%E6%96%87%2F%25+done.bin&override=false',
		})
	})

	it('fails before starting when File Browser cannot create the TUS upload', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response('missing parent', {
			status: 404,
			statusText: 'Not Found',
		}))

		await expect(uploadTus({
			endpoint: 'https://viewer.example.test/viewer-files/tus?viewer_session_id=vs_1&path=%2Fdata.bin',
			fetcher,
			headers: { Authorization: 'Bearer user-auth' },
			file: new Blob(['payload']),
			path: '/data.bin',
		})).rejects.toMatchObject({
			code: 'TUS_UPLOAD_FAILED',
			message: 'missing parent',
			status: 404,
		})

		expect(tusState.start).not.toHaveBeenCalled()
	})

	it('does not retry File Browser conflict responses', async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 201 }))

		await uploadTus({
			endpoint: 'https://viewer.example.test/viewer-files/tus?viewer_session_id=vs_1&path=%2Fdata.bin',
			fetcher,
			headers: { Authorization: 'Bearer user-auth' },
			file: new Blob(['payload']),
			path: '/data.bin',
		})

		const shouldRetry = tusState.options?.onShouldRetry as (error: {
			originalResponse?: { getStatus: () => number }
		}) => boolean

		expect(shouldRetry({ originalResponse: { getStatus: () => 409 } })).toBe(false)
		expect(shouldRetry({ originalResponse: { getStatus: () => 500 } })).toBe(true)
	})
})
