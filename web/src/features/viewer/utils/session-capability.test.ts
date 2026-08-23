import { describe, expect, it } from 'vitest'

import { ViewerApiError } from '@/features/viewer/api/viewer-error'
import { pvcFixture, viewerSessionFixture } from '@/features/viewer/test/fakes'
import { deriveSessionCapability } from '@/features/viewer/utils/session-capability'

describe('session capability helpers', () => {
	it('hides session navigation before a PVC is selected', () => {
		const capability = deriveSessionCapability({
			error: null,
			isReconnecting: false,
			manualCloseKind: null,
			selectedPVC: null,
			session: null,
			status: 'idle',
		})

		expect(capability.kind).toBe('none')
		expect(capability.canShowSessionNavigation).toBe(false)
		expect(capability.canShowFileList).toBe(false)
	})

	it('shows pod-only state while the viewer session is not token-ready', () => {
		const capability = deriveSessionCapability({
			error: null,
			isReconnecting: false,
			manualCloseKind: null,
			selectedPVC: pvcFixture(),
			session: viewerSessionFixture({ status: 'creating', token_ready: false }),
			status: 'polling',
		})

		expect(capability.kind).toBe('pod-only')
		expect(capability.canShowSessionNavigation).toBe(true)
		expect(capability.canShowFileList).toBe(false)
		expect(capability.canUseFiles).toBe(false)
	})

	it('enables file capabilities when the viewer session is ready', () => {
		const capability = deriveSessionCapability({
			error: null,
			isReconnecting: false,
			manualCloseKind: null,
			selectedPVC: pvcFixture(),
			session: viewerSessionFixture({ status: 'ready', token_ready: true }),
			status: 'ready',
		})

		expect(capability.kind).toBe('viewer-ready')
		expect(capability.canShowFileList).toBe(true)
		expect(capability.canUseFiles).toBe(true)
	})

	it('keeps the file list visible but disabled during reconnect', () => {
		const capability = deriveSessionCapability({
			error: new ViewerApiError({ code: 'VIEWER_SESSION_NOT_FOUND', message: 'lost', status: 404 }),
			isReconnecting: true,
			manualCloseKind: null,
			selectedPVC: pvcFixture(),
			session: viewerSessionFixture({ status: 'ready', token_ready: true }),
			status: 'failed',
		})

		expect(capability.kind).toBe('viewer-reconnecting')
		expect(capability.canShowFileList).toBe(true)
		expect(capability.canUseFiles).toBe(false)
	})

	it('prevents automatic recovery after a manual close', () => {
		const capability = deriveSessionCapability({
			error: null,
			isReconnecting: false,
			manualCloseKind: 'pod',
			selectedPVC: pvcFixture(),
			session: viewerSessionFixture({ status: 'closed' }),
			status: 'idle',
		})

		expect(capability.kind).toBe('manual-closed')
		expect(capability.canShowSessionNavigation).toBe(false)
		expect(capability.canShowFileList).toBe(false)
	})
})
