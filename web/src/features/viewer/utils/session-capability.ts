import type { ViewerApiError } from '@/features/viewer/api/viewer-error'
import type { PVC, ViewerSession } from '@/features/viewer/types/viewer'

export type ViewerFlowStatus = 'idle' | 'creating' | 'polling' | 'ready' | 'failed'
export type ManualCloseKind = 'viewer' | 'pod'
export type SessionCapabilityKind
	= | 'none'
		| 'starting-pod'
		| 'pod-only'
		| 'viewer-ready'
		| 'viewer-reconnecting'
		| 'manual-closed'
		| 'failed'

export interface SessionCapabilityInput {
	error: ViewerApiError | null
	isReconnecting: boolean
	manualCloseKind: ManualCloseKind | null
	selectedPVC: PVC | null
	session: ViewerSession | null
	status: ViewerFlowStatus
}

export interface SessionCapability {
	canShowFileList: boolean
	canShowSessionNavigation: boolean
	canUseFiles: boolean
	error: ViewerApiError | null
	kind: SessionCapabilityKind
	manualCloseKind: ManualCloseKind | null
	messageKey: string
}

export function deriveSessionCapability({
	error,
	isReconnecting,
	manualCloseKind,
	selectedPVC,
	session,
	status,
}: SessionCapabilityInput): SessionCapability {
	if (!selectedPVC) {
		return capability('none', 'viewer.noSelection', false, false, false, error, manualCloseKind)
	}

	if (manualCloseKind) {
		return capability('manual-closed', 'files.manualClosed', manualCloseKind === 'viewer', false, false, error, manualCloseKind)
	}

	if (isReconnecting) {
		return capability('viewer-reconnecting', 'files.reconnecting', true, true, false, error, manualCloseKind)
	}

	if (session?.status === 'ready' && session.token_ready) {
		return capability('viewer-ready', 'files.ready', true, true, true, error, manualCloseKind)
	}

	if (status === 'failed') {
		return capability('failed', 'files.viewerUnavailable', true, false, false, error, manualCloseKind)
	}

	if (session) {
		return capability('pod-only', 'common.loading', true, false, false, error, manualCloseKind)
	}

	if (status === 'creating' || status === 'polling') {
		return capability('starting-pod', 'common.loading', true, false, false, error, manualCloseKind)
	}

	return capability('starting-pod', 'common.loading', true, false, false, error, manualCloseKind)
}

function capability(
	kind: SessionCapabilityKind,
	messageKey: string,
	canShowSessionNavigation: boolean,
	canShowFileList: boolean,
	canUseFiles: boolean,
	error: ViewerApiError | null,
	manualCloseKind: ManualCloseKind | null,
): SessionCapability {
	return {
		canShowFileList,
		canShowSessionNavigation,
		canUseFiles,
		error,
		kind,
		manualCloseKind,
		messageKey,
	}
}
