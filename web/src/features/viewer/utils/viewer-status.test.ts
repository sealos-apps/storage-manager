import { describe, expect, it } from 'vitest'

import { pvcFixture, viewerSessionFixture } from '@/features/viewer/test/fakes'
import { canLaunchViewer, isReadOnlyPVC, isViewerSessionReady, isViewerSessionTerminal, launchBlockReason, pvcIdentity } from '@/features/viewer/utils/viewer-status'

describe('viewer status helpers', () => {
	it('derives PVC identity and launch capability', () => {
		const pvc = pvcFixture({
			name: 'data',
			namespace: 'prod',
			uid: 'uid-1',
		})

		expect(pvcIdentity(pvc)).toEqual({
			namespace: 'prod',
			pvcName: 'data',
			uid: 'uid-1',
		})
		expect(canLaunchViewer(pvc)).toBe(true)
	})

	it('returns launch block reasons for unsupported PVCs', () => {
		const pvc = pvcFixture({
			reason: 'ReadWriteOncePod is unsupported',
			viewer_supported: false,
		})

		expect(canLaunchViewer(pvc)).toBe(false)
		expect(launchBlockReason(pvc)).toBe('ReadWriteOncePod is unsupported')
	})

	it('classifies session and readonly state', () => {
		expect(isViewerSessionReady(viewerSessionFixture({ status: 'ready', token_ready: true }))).toBe(true)
		expect(isViewerSessionTerminal(viewerSessionFixture({ status: 'failed' }))).toBe(true)
		expect(isReadOnlyPVC(pvcFixture({ viewer_mode: 'readonly' }))).toBe(true)
	})
})
