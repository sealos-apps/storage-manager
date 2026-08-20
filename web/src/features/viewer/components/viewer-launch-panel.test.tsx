import { waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { ViewerLaunchPanel } from '@/features/viewer/components/viewer-launch-panel'
import { createFakeViewerAPI, pvcFixture, viewerSessionFixture } from '@/features/viewer/test/fakes'
import { renderWithProviders } from '@/test/render'

describe('viewerLaunchPanel', () => {
	it('starts a viewer session only once for the same auto start key', async () => {
		const createViewerSession = vi.fn().mockResolvedValue(
			viewerSessionFixture({
				id: 'vs_1',
				status: 'creating',
				token_ready: false,
			}),
		)
		const api = createFakeViewerAPI({ createViewerSession })
		const pvc = pvcFixture()

		const { rerender } = renderWithProviders(
			<ViewerLaunchPanel
				api={api}
				autoStartKey="pvc-uid:1"
				pvc={pvc}
			/>,
		)

		await waitFor(() => expect(createViewerSession).toHaveBeenCalledTimes(1))

		rerender(
			<ViewerLaunchPanel
				api={api}
				autoStartKey="pvc-uid:1"
				onFlowChange={() => undefined}
				pvc={pvc}
			/>,
		)
		await Promise.resolve()

		expect(createViewerSession).toHaveBeenCalledTimes(1)
	})
})
