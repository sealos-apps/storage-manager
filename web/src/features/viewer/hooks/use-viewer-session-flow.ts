import type { ViewerApiError } from '@/features/viewer/api/viewer-error'
import type { ViewerAPI, ViewerSelection, ViewerSession } from '@/features/viewer/types/viewer'
import type { ManualCloseKind } from '@/features/viewer/utils/session-capability'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'

import { viewerApi } from '@/features/viewer/api/viewer-api'
import { isMissingSessionError, normalizeViewerError } from '@/features/viewer/api/viewer-error'
import { createViewerSessionMutationOptions } from '@/features/viewer/api/viewer-mutations'
import { viewerSessionQueryOptions } from '@/features/viewer/api/viewer-query-options'

type FlowStatus = 'idle' | 'creating' | 'polling' | 'ready' | 'failed'

interface UseViewerSessionFlowInput {
	api?: ViewerAPI
	maxAutoRecoveries?: number
	pollIntervalMs?: number
}

export interface ViewerSessionFlow {
	error: ViewerApiError | null
	isManualClosed: boolean
	isReconnecting: boolean
	manualCloseKind: ManualCloseKind | null
	recover: (error?: unknown) => Promise<void>
	registerManualClose: (kind: ManualCloseKind) => void
	reset: () => void
	session: ViewerSession | null
	start: (pvc: ViewerSelection) => Promise<void>
	status: FlowStatus
}

function shouldPollStatus(status: ViewerSession['status']) {
	return status === 'creating' || status === 'active'
}

function isTerminalFailureStatus(status: ViewerSession['status']) {
	return status === 'failed' || status === 'expired' || status === 'closed'
}

function shouldRecoverSession(error: ViewerApiError) {
	return isMissingSessionError(error)
}

export function useViewerSessionFlow({
	api = viewerApi,
	maxAutoRecoveries = 1,
	pollIntervalMs = 2_000,
}: UseViewerSessionFlowInput = {}): ViewerSessionFlow {
	const queryClient = useQueryClient()
	const [error, setError] = useState<ReturnType<typeof normalizeViewerError> | null>(null)
	const [selectedPVC, setSelectedPVC] = useState<ViewerSelection | null>(null)
	const [session, setSession] = useState<ViewerSession | null>(null)
	const [status, setStatus] = useState<FlowStatus>('idle')
	const [manualCloseKind, setManualCloseKind] = useState<ManualCloseKind | null>(null)
	const [isReconnecting, setIsReconnecting] = useState(false)
	const createViewerSession = useMutation(createViewerSessionMutationOptions(queryClient, api))
	const createViewerSessionRef = useRef(createViewerSession.mutateAsync)
	const manualCloseKindRef = useRef(manualCloseKind)
	const autoRecoveryCountRef = useRef(0)
	const selectedPVCRef = useRef(selectedPVC)
	const pollingSessionID = session?.id ?? null
	const pollingSessionStatus = session?.status ?? null
	useEffect(() => {
		createViewerSessionRef.current = createViewerSession.mutateAsync
		manualCloseKindRef.current = manualCloseKind
		selectedPVCRef.current = selectedPVC
	}, [createViewerSession.mutateAsync, manualCloseKind, selectedPVC])

	const createForPVC = useCallback(async (pvc: ViewerSelection) => {
		setStatus('creating')
		setError(null)
		setIsReconnecting(false)
		setManualCloseKind(null)
		const nextSession = await createViewerSessionRef.current({
			namespace: pvc.namespace,
			pvcName: pvc.pvcName,
		})
		setSession(nextSession)
		setStatus(nextSession.status === 'ready' ? 'ready' : 'polling')
	}, [])

	const start = useCallback(async (pvc: ViewerSelection) => {
		autoRecoveryCountRef.current = 0
		setSelectedPVC(pvc)
		setManualCloseKind(null)
		setIsReconnecting(false)
		try {
			await createForPVC(pvc)
		}
		catch (caught) {
			setError(normalizeViewerError(caught))
			setStatus('failed')
		}
	}, [createForPVC])

	const reset = useCallback(() => {
		setError(null)
		setIsReconnecting(false)
		setManualCloseKind(null)
		setSelectedPVC(null)
		setSession(null)
		setStatus('idle')
		autoRecoveryCountRef.current = 0
	}, [])

	const registerManualClose = useCallback((kind: ManualCloseKind) => {
		setManualCloseKind(kind)
		setIsReconnecting(false)
		setError(null)
		if (kind === 'pod') {
			setSelectedPVC(null)
			setSession(null)
		}
		else if (session) {
			setSession({ ...session, status: 'closed' })
		}
		setStatus('idle')
		autoRecoveryCountRef.current = 0
	}, [session])

	const recover = useCallback(async (caught?: unknown) => {
		const currentPVC = selectedPVCRef.current
		if (!currentPVC || manualCloseKindRef.current) {
			return
		}
		const nextError = caught ? normalizeViewerError(caught) : null
		if (nextError) {
			setError(nextError)
		}
		if (nextError && shouldRecoverSession(nextError)) {
			if (autoRecoveryCountRef.current >= maxAutoRecoveries) {
				setIsReconnecting(false)
				setStatus('failed')
				setSession(null)
				return
			}
			autoRecoveryCountRef.current += 1
		}
		setIsReconnecting(true)
		try {
			await createForPVC(currentPVC)
		}
		catch (createError) {
			setError(normalizeViewerError(createError))
			setStatus('failed')
		}
		finally {
			setIsReconnecting(false)
		}
	}, [createForPVC, maxAutoRecoveries])

	useEffect(() => {
		if (!pollingSessionID || !pollingSessionStatus || !shouldPollStatus(pollingSessionStatus)) {
			return undefined
		}

		const id = window.setInterval(() => {
			void queryClient.fetchQuery(viewerSessionQueryOptions({
				api,
				viewerSessionID: pollingSessionID,
			}))
				.then((nextSession) => {
					setSession(nextSession)
					if (nextSession.status === 'ready') {
						setStatus('ready')
					}
					else if (isTerminalFailureStatus(nextSession.status)) {
						setStatus('failed')
					}
				})
				.catch(async (caught) => {
					const nextError = normalizeViewerError(caught)
					if (shouldRecoverSession(nextError) && selectedPVCRef.current) {
						try {
							await recover(nextError)
						}
						catch (createError) {
							setError(normalizeViewerError(createError))
							setStatus('failed')
						}
						return
					}
					setError(nextError)
					setStatus('failed')
				})
		}, pollIntervalMs)

		return () => window.clearInterval(id)
	}, [api, pollIntervalMs, pollingSessionID, pollingSessionStatus, queryClient, recover])

	return {
		error,
		isManualClosed: manualCloseKind !== null,
		isReconnecting,
		manualCloseKind,
		recover,
		registerManualClose,
		reset,
		session,
		start,
		status,
	}
}
