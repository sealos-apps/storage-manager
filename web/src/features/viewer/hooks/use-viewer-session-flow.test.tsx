import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ViewerApiError } from '@/features/viewer/api/viewer-error'
import { useViewerSessionFlow } from '@/features/viewer/hooks/use-viewer-session-flow'
import { createFakeViewerAPI, viewerSessionFixture, viewerTokenFixture } from '@/features/viewer/test/fakes'

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

		const { result } = renderHook(() =>
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

		const { result } = renderHook(() =>
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
})
