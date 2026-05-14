import type { PVC, ViewerSession } from '@/features/viewer/types/viewer'

export function pvcIdentity(pvc: PVC) {
	return {
		namespace: pvc.namespace,
		pvcName: pvc.name,
		uid: pvc.uid,
	}
}

export function canLaunchViewer(pvc: PVC) {
	return pvc.viewer_supported && pvc.reason === ''
}

export function launchBlockReason(pvc: PVC) {
	if (!pvc.viewer_supported) {
		return pvc.reason || 'UNSUPPORTED_ACCESS_MODE'
	}
	if (pvc.viewer_scheduling.reason) {
		return pvc.viewer_scheduling.reason
	}
	return pvc.reason
}

export function isViewerSessionReady(session: ViewerSession | null | undefined) {
	return session?.status === 'ready' && session.token_ready
}

export function isViewerSessionTerminal(session: ViewerSession | null | undefined) {
	return session?.status === 'failed'
		|| session?.status === 'closed'
		|| session?.status === 'expired'
}

export function isReadOnlyPVC(pvc: PVC) {
	return pvc.viewer_mode === 'readonly'
}
