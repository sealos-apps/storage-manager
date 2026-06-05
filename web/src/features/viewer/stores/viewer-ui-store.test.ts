import { describe, expect, it } from 'vitest'

import { initialViewerUIState, viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'

describe('viewerUIStore', () => {
	it('tracks namespace, selected PVC, active session, and reset', () => {
		viewerUIStore.actions.reset()

		viewerUIStore.actions.syncContextNamespace('prod')
		viewerUIStore.actions.setSearch('mysql')
		viewerUIStore.actions.selectPVC({
			namespace: 'prod',
			pvcName: 'mysql-data',
			uid: 'uid-1',
		})
		viewerUIStore.actions.setActiveSession('vs_1', 'ps_1')

		expect(viewerUIStore.state).toMatchObject({
			activePodSessionID: 'ps_1',
			activeViewerSessionID: 'vs_1',
			namespace: 'prod',
			search: 'mysql',
			selectedPVC: {
				namespace: 'prod',
				pvcName: 'mysql-data',
				uid: 'uid-1',
			},
			view: 'files',
		})

		viewerUIStore.actions.reset()

		expect(viewerUIStore.state).toEqual(initialViewerUIState)
	})

	it('keeps selection when syncing the same context namespace', () => {
		viewerUIStore.actions.reset()

		viewerUIStore.actions.syncContextNamespace('prod')
		viewerUIStore.actions.selectPVC({
			namespace: 'prod',
			pvcName: 'mysql-data',
			uid: 'uid-1',
		})
		viewerUIStore.actions.setActiveSession('vs_1', 'ps_1')
		viewerUIStore.actions.syncContextNamespace('prod')

		expect(viewerUIStore.state).toMatchObject({
			activePodSessionID: 'ps_1',
			activeViewerSessionID: 'vs_1',
			namespace: 'prod',
			selectedPVC: {
				pvcName: 'mysql-data',
			},
			view: 'files',
		})
	})

	it('clears active session state when admin selects another namespace', () => {
		viewerUIStore.actions.reset()

		viewerUIStore.actions.syncContextNamespace('prod')
		viewerUIStore.actions.selectPVC({
			namespace: 'prod',
			pvcName: 'mysql-data',
			uid: 'uid-1',
		})
		viewerUIStore.actions.setActiveSession('vs_1', 'ps_1')
		viewerUIStore.actions.setNamespace('kube-system')

		expect(viewerUIStore.state).toMatchObject({
			activePodSessionID: null,
			activeViewerSessionID: null,
			namespace: 'kube-system',
			selectedPVC: null,
			view: 'volumes',
		})
	})
})
