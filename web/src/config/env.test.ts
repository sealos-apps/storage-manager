import { afterEach, describe, expect, it, vi } from 'vitest'

describe('frontend environment parsing', () => {
	afterEach(() => {
		delete window.__SEALOS_STORAGE_MANAGER_CONFIG__
		vi.resetModules()
		vi.unstubAllEnvs()
	})

	it('uses defaults when runtime config and env vars are absent', async () => {
		const { env } = await import('@/config/env')

		expect(env.apiBaseUrl).toBe('/api')
		expect(env.fileUploadTusThresholdBytes).toBe(32 * 1024 * 1024)
		expect(env.fileUploadTusChunkBytes).toBe(8 * 1024 * 1024)
		expect(env.fileUploadTusRetryCount).toBe(5)
	})

	it('uses runtime config before Vite env vars in production mode', async () => {
		vi.stubEnv('DEV', false)
		vi.stubEnv('VITE_API_BASE_URL', 'https://build.example.com/api')
		window.__SEALOS_STORAGE_MANAGER_CONFIG__ = {
			apiBaseUrl: '/runtime-api',
			fileUploadTusChunkBytes: 256,
			fileUploadTusRetryCount: '3',
			fileUploadTusThresholdBytes: '2048',
		}

		const { env } = await import('@/config/env')

		expect(env.apiBaseUrl).toBe('/runtime-api')
		expect(env.fileUploadTusThresholdBytes).toBe(2048)
		expect(env.fileUploadTusChunkBytes).toBe(256)
		expect(env.fileUploadTusRetryCount).toBe(3)
	})

	it('uses Vite API base URL before runtime config in dev mode', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_API_BASE_URL', 'http://localhost:4000')
		window.__SEALOS_STORAGE_MANAGER_CONFIG__ = {
			apiBaseUrl: '/api',
		}

		const { env } = await import('@/config/env')

		expect(env.apiBaseUrl).toBe('http://localhost:4000')
	})

	it('uses Vite env vars when runtime config is absent', async () => {
		vi.stubEnv('VITE_API_BASE_URL', 'https://build.example.com/api')

		const { env } = await import('@/config/env')

		expect(env.apiBaseUrl).toBe('https://build.example.com/api')
		expect(env.fileUploadTusThresholdBytes).toBe(32 * 1024 * 1024)
		expect(env.fileUploadTusChunkBytes).toBe(8 * 1024 * 1024)
		expect(env.fileUploadTusRetryCount).toBe(5)
	})

	it('falls back from invalid runtime upload values to defaults', async () => {
		window.__SEALOS_STORAGE_MANAGER_CONFIG__ = {
			fileUploadTusChunkBytes: 0,
			fileUploadTusRetryCount: 'invalid',
			fileUploadTusThresholdBytes: -1,
		}

		const { env } = await import('@/config/env')

		expect(env.fileUploadTusThresholdBytes).toBe(32 * 1024 * 1024)
		expect(env.fileUploadTusChunkBytes).toBe(8 * 1024 * 1024)
		expect(env.fileUploadTusRetryCount).toBe(5)
	})
})
