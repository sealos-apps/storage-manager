import { act } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ViewerApiError } from '@/features/viewer/api/viewer-error'
import { useViewerSessionFlow } from '@/features/viewer/hooks/use-viewer-session-flow'
import { createFakeViewerAPI, viewerSessionFixture, viewerTokenFixture } from '@/features/viewer/test/fakes'
import { renderHookWithProviders } from '@/test/render'

describe('useViewerSessionFlow', () => {
	afterEach(() => {
		vi.useRealTimers()
	})

	it('creates a session, polls until ready, and issues a token', async () => {
		vi.useFakeTimers()
		const getViewerSession = vi
			.fn()
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_1', status: 'ready', token_ready: true }))
		const issueViewerToken = vi.fn().mockResolvedValue(viewerTokenFixture({ viewer_session_id: 'vs_1' }))
		const api = createFakeViewerAPI({
			createViewerSession: async () => viewerSessionFixture({ id: 'vs_1', status: 'creating' }),
			getViewerSession,
			issueViewerToken,
		})

		const { result } = renderHookWithProviders(() =>
			useViewerSessionFlow({ api, pollIntervalMs: 1000 }),
		)

		await act(async () => {
			await result.current.start({
				namespace: 'default',
				pvcName: 'data',
				uid: 'uid',
			})
		})

		expect(result.current.status).toBe('polling')

		await act(async () => {
			await vi.advanceTimersByTimeAsync(1000)
		})

		await vi.waitFor(() => expect(result.current.status).toBe('ready'))
		expect(issueViewerToken).toHaveBeenCalledWith('vs_1')
		expect(result.current.token?.token).toBe('fb-token')
	})

	it('recreates a session when backend reports VIEWER_SESSION_NOT_FOUND', async () => {
		vi.useFakeTimers()
		const createViewerSession = vi
			.fn()
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_old', status: 'creating' }))
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_new', status: 'creating' }))
		const getViewerSession = vi.fn().mockRejectedValue(new ViewerApiError({
			code: 'VIEWER_SESSION_NOT_FOUND',
			message: 'lost',
			status: 404,
		}))
		const api = createFakeViewerAPI({
			createViewerSession,
			getViewerSession,
		})

		const { result } = renderHookWithProviders(() =>
			useViewerSessionFlow({ api, pollIntervalMs: 1000 }),
		)

		await act(async () => {
			await result.current.start({
				namespace: 'default',
				pvcName: 'data',
				uid: 'uid',
			})
		})

		await act(async () => {
			await vi.advanceTimersByTimeAsync(1000)
		})

		await vi.waitFor(() => expect(result.current.session?.id).toBe('vs_new'))
		expect(createViewerSession).toHaveBeenCalledTimes(2)
	})

	it('recreates a session when backend reports POD_SESSION_NOT_FOUND', async () => {
		vi.useFakeTimers()
		const createViewerSession = vi
			.fn()
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_old', status: 'creating' }))
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_new', status: 'creating' }))
		const getViewerSession = vi.fn().mockRejectedValue(new ViewerApiError({
			code: 'POD_SESSION_NOT_FOUND',
			message: 'pod lost',
			status: 404,
		}))
		const api = createFakeViewerAPI({
			createViewerSession,
			getViewerSession,
		})

		const { result } = renderHookWithProviders(() =>
			useViewerSessionFlow({ api, pollIntervalMs: 1000 }),
		)

		await act(async () => {
			await result.current.start({
				namespace: 'default',
				pvcName: 'data',
				uid: 'uid',
			})
		})

		await act(async () => {
			await vi.advanceTimersByTimeAsync(1000)
		})

		await vi.waitFor(() => expect(result.current.session?.id).toBe('vs_new'))
		expect(result.current.error).toBeNull()
		expect(createViewerSession).toHaveBeenCalledTimes(2)
	})

	it('recreates a session when token issuance reports POD_SESSION_NOT_FOUND', async () => {
		const createViewerSession = vi
			.fn()
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_old', status: 'ready', token_ready: true }))
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_new', status: 'creating' }))
		const issueViewerToken = vi.fn().mockRejectedValue(new ViewerApiError({
			code: 'POD_SESSION_NOT_FOUND',
			message: 'Pod session no longer exists',
			status: 404,
		}))
		const api = createFakeViewerAPI({
			createViewerSession,
			issueViewerToken,
		})

		const { result } = renderHookWithProviders(() =>
			useViewerSessionFlow({ api }),
		)

		await act(async () => {
			await result.current.start({
				namespace: 'default',
				pvcName: 'data',
				uid: 'uid',
			})
		})

		await vi.waitFor(() => expect(result.current.session?.id).toBe('vs_new'))
		expect(issueViewerToken).toHaveBeenCalledWith('vs_old')
		expect(createViewerSession).toHaveBeenCalledTimes(2)
	})

	it('stops automatic recovery after the retry limit', async () => {
		const createViewerSession = vi
			.fn()
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_first', status: 'ready', token_ready: true }))
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_second', status: 'ready', token_ready: true }))
		const issueViewerToken = vi.fn().mockRejectedValue(new ViewerApiError({
			code: 'POD_SESSION_NOT_FOUND',
			message: 'Pod session no longer exists',
			status: 404,
		}))
		const api = createFakeViewerAPI({
			createViewerSession,
			issueViewerToken,
		})

		const { result } = renderHookWithProviders(() =>
			useViewerSessionFlow({ api, maxAutoRecoveries: 1 }),
		)

		await act(async () => {
			await result.current.start({
				namespace: 'default',
				pvcName: 'data',
				uid: 'uid',
			})
		})

		await vi.waitFor(() => expect(result.current.status).toBe('failed'))
		expect(result.current.error?.code).toBe('POD_SESSION_NOT_FOUND')
		expect(createViewerSession).toHaveBeenCalledTimes(2)
		expect(issueViewerToken).toHaveBeenCalledTimes(2)
	})

	it('does not recreate sessions when File Browser token issuance fails', async () => {
		const createViewerSession = vi
			.fn()
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_1', status: 'ready', token_ready: true }))
		const issueViewerToken = vi.fn().mockRejectedValue(new ViewerApiError({
			code: 'FILEBROWSER_LOGIN_FAILED',
			message: 'filebrowser login returned status 502',
			status: 502,
		}))
		const api = createFakeViewerAPI({
			createViewerSession,
			issueViewerToken,
		})

		const { result } = renderHookWithProviders(() =>
			useViewerSessionFlow({ api }),
		)

		await act(async () => {
			await result.current.start({
				namespace: 'default',
				pvcName: 'data',
				uid: 'uid',
			})
		})

		await vi.waitFor(() => expect(result.current.status).toBe('failed'))
		expect(result.current.error?.code).toBe('FILEBROWSER_LOGIN_FAILED')
		expect(createViewerSession).toHaveBeenCalledTimes(1)
		expect(issueViewerToken).toHaveBeenCalledTimes(1)
	})

	it('does not recover after a manual close is registered', async () => {
		const createViewerSession = vi.fn().mockResolvedValue(viewerSessionFixture({ id: 'vs_1', status: 'creating' }))
		const api = createFakeViewerAPI({ createViewerSession })

		const { result } = renderHookWithProviders(() =>
			useViewerSessionFlow({ api, pollIntervalMs: 1000 }),
		)

		await act(async () => {
			await result.current.start({
				namespace: 'default',
				pvcName: 'data',
				uid: 'uid',
			})
		})
		act(() => result.current.registerManualClose('viewer'))
		await act(async () => {
			await result.current.recover(new Error('lost'))
		})

		expect(result.current.isManualClosed).toBe(true)
		expect(result.current.manualCloseKind).toBe('viewer')
		expect(result.current.token).toBeNull()
		expect(createViewerSession).toHaveBeenCalledTimes(1)
	})

	it('does not issue duplicate tokens when session polling causes rerenders', async () => {
		vi.useFakeTimers()
		const getViewerSession = vi
			.fn()
			.mockResolvedValue(viewerSessionFixture({ id: 'vs_1', status: 'ready', token_ready: true }))
		const issueViewerToken = vi.fn().mockResolvedValue(viewerTokenFixture({ viewer_session_id: 'vs_1' }))
		const api = createFakeViewerAPI({
			createViewerSession: async () => viewerSessionFixture({ id: 'vs_1', status: 'creating' }),
			getViewerSession,
			issueViewerToken,
		})

		const { result, rerender } = renderHookWithProviders(() =>
			useViewerSessionFlow({ api, pollIntervalMs: 1000 }),
		)

		await act(async () => {
			await result.current.start({
				namespace: 'default',
				pvcName: 'data',
				uid: 'uid',
			})
		})
		await act(async () => {
			await vi.advanceTimersByTimeAsync(1000)
		})
		await vi.waitFor(() => expect(result.current.status).toBe('ready'))

		rerender()
		await act(async () => {
			await vi.advanceTimersByTimeAsync(0)
		})

		expect(issueViewerToken).toHaveBeenCalledTimes(1)
	})
})
