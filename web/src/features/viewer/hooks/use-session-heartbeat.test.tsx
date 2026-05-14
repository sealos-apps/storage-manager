import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useSessionHeartbeat } from '@/features/viewer/hooks/use-session-heartbeat'
import { createFakeViewerAPI } from '@/features/viewer/test/fakes'

describe('useSessionHeartbeat', () => {
	afterEach(() => {
		vi.useRealTimers()
	})

	it('sends heartbeat immediately and at the configured interval', async () => {
		vi.useFakeTimers()
		const heartbeatViewerSession = vi.fn().mockResolvedValue({})
		const api = createFakeViewerAPI({ heartbeatViewerSession })

		const { unmount } = renderHook(() =>
			useSessionHeartbeat({
				api,
				enabled: true,
				intervalMs: 1000,
				viewerSessionID: 'vs_1',
			}),
		)

		expect(heartbeatViewerSession).toHaveBeenCalledTimes(1)

		await vi.advanceTimersByTimeAsync(2500)
		expect(heartbeatViewerSession).toHaveBeenCalledTimes(3)

		unmount()
		await vi.advanceTimersByTimeAsync(1000)
		expect(heartbeatViewerSession).toHaveBeenCalledTimes(3)
	})
})
