import { renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { useBeforeUnloadCloseSession } from '@/features/viewer/hooks/use-before-unload-close-session'
import { createFakeViewerAPI } from '@/features/viewer/test/fakes'

describe('useBeforeUnloadCloseSession', () => {
	it('closes the active viewer session on pagehide', () => {
		const closeViewerSession = vi.fn().mockResolvedValue({})
		const api = createFakeViewerAPI({ closeViewerSession })

		const { unmount } = renderHook(() =>
			useBeforeUnloadCloseSession({
				api,
				enabled: true,
				viewerSessionID: 'vs_1',
			}),
		)

		window.dispatchEvent(new Event('pagehide'))
		expect(closeViewerSession).toHaveBeenCalledWith('vs_1')

		unmount()
		window.dispatchEvent(new Event('pagehide'))
		expect(closeViewerSession).toHaveBeenCalledTimes(1)
	})
})
