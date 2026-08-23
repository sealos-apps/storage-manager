import { act } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ViewerApiError } from '@/features/viewer/api/viewer-error'
import { useViewerSessionFlow } from '@/features/viewer/hooks/use-viewer-session-flow'
import { createFakeViewerAPI, viewerSessionFixture } from '@/features/viewer/test/fakes'
import { renderHookWithProviders } from '@/test/render'

describe('useViewerSessionFlow', () => {
	afterEach(() => {
		vi.useRealTimers()
	})

	it('creates a session, polls until ready, and never requests a browser token', async () => {
		vi.useFakeTimers()
		const getViewerSession = vi
			.fn()
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_1', status: 'ready', token_ready: true }))
		const issueViewerToken = vi.fn()
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
		expect(result.current.session?.id).toBe('vs_1')
		expect(issueViewerToken).not.toHaveBeenCalled()
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
		const api = createFakeViewerAPI({ createViewerSession, getViewerSession })

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

	it('stops automatic recovery after the retry limit', async () => {
		const createViewerSession = vi
			.fn()
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_first', status: 'creating' }))
			.mockResolvedValueOnce(viewerSessionFixture({ id: 'vs_second', status: 'creating' }))
		const api = createFakeViewerAPI({ createViewerSession })

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
		await act(async () => {
			await result.current.recover(new ViewerApiError({
				code: 'POD_SESSION_NOT_FOUND',
				message: 'pod session no longer exists',
				status: 404,
			}))
		})

		expect(result.current.session?.id).toBe('vs_second')
		await act(async () => {
			await result.current.recover(new ViewerApiError({
				code: 'POD_SESSION_NOT_FOUND',
				message: 'pod session no longer exists',
				status: 404,
			}))
		})
		expect(result.current.status).toBe('failed')
		expect(result.current.session).toBeNull()
		expect(createViewerSession).toHaveBeenCalledTimes(2)
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
		expect(createViewerSession).toHaveBeenCalledTimes(1)
	})
})
