import type { ViewerSessionFlow } from '@/features/viewer/hooks/use-viewer-session-flow'
import type { PVC, ViewerAPI, ViewerToken } from '@/features/viewer/types/viewer'

import { useEffect, useRef } from 'react'
import { useHasActiveUploadsForSession } from '@/features/file-manager/stores/upload-store'
import { viewerApi } from '@/features/viewer/api/viewer-api'
import { useBeforeUnloadCloseSession } from '@/features/viewer/hooks/use-before-unload-close-session'
import { useSessionHeartbeat } from '@/features/viewer/hooks/use-session-heartbeat'
import { useViewerSessionFlow } from '@/features/viewer/hooks/use-viewer-session-flow'
import { viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'
import { pvcIdentity } from '@/features/viewer/utils/viewer-status'

export interface ViewerFlowSnapshot {
	error: ViewerSessionFlow['error']
	isReconnecting: ViewerSessionFlow['isReconnecting']
	manualCloseKind: ViewerSessionFlow['manualCloseKind']
	recover: ViewerSessionFlow['recover']
	registerManualClose: ViewerSessionFlow['registerManualClose']
	session: ViewerSessionFlow['session']
	status: ViewerSessionFlow['status']
}

interface ViewerLaunchPanelProps {
	api?: ViewerAPI
	autoStartKey?: string | null
	onFlowChange?: (flow: ViewerFlowSnapshot) => void
	onSessionStatusChange?: (status: string) => void
	pvc: PVC | null
	setToken: (token: ViewerToken | null) => void
}

export function ViewerLaunchPanel({
	api = viewerApi,
	autoStartKey,
	onFlowChange,
	onSessionStatusChange,
	pvc,
	setToken,
}: ViewerLaunchPanelProps) {
	const flow = useViewerSessionFlow({ api })
	const active = flow.status === 'ready'
	const startFlow = flow.start
	const recoverFlow = flow.recover
	const startFlowRef = useRef(startFlow)
	const startedAutoKeyRef = useRef<string | null>(null)
	const hasActiveUpload = useHasActiveUploadsForSession({
		podSessionID: flow.session?.pod_session_id ?? null,
		viewerSessionID: flow.session?.id ?? null,
	})

	useEffect(() => {
		startFlowRef.current = startFlow
	}, [startFlow])

	useSessionHeartbeat({
		api,
		enabled: active && !hasActiveUpload,
		onError: error => void recoverFlow(error),
		viewerSessionID: flow.session?.id ?? null,
	})
	useBeforeUnloadCloseSession({
		api,
		enabled: Boolean(flow.session?.id),
		hasActiveUpload,
		viewerSessionID: flow.session?.id ?? null,
	})

	useEffect(() => {
		if (flow.session) {
			viewerUIStore.actions.setActiveSession(flow.session.id, flow.session.pod_session_id)
		}
	}, [flow.session])

	useEffect(() => {
		setToken(flow.token)
	}, [flow.token, setToken])

	useEffect(() => {
		onSessionStatusChange?.(flow.status)
	}, [flow.status, onSessionStatusChange])

	useEffect(() => {
		onFlowChange?.({
			error: flow.error,
			isReconnecting: flow.isReconnecting,
			manualCloseKind: flow.manualCloseKind,
			recover: flow.recover,
			registerManualClose: flow.registerManualClose,
			session: flow.session,
			status: flow.status,
		})
	}, [
		flow.error,
		flow.isReconnecting,
		flow.manualCloseKind,
		flow.recover,
		flow.registerManualClose,
		flow.session,
		flow.status,
		onFlowChange,
	])

	useEffect(() => {
		if (!pvc || !autoStartKey) {
			startedAutoKeyRef.current = null
			return
		}
		if (startedAutoKeyRef.current === autoStartKey) {
			return
		}
		startedAutoKeyRef.current = autoStartKey
		void startFlowRef.current(pvcIdentity(pvc))
	}, [autoStartKey, pvc])

	return null
}
