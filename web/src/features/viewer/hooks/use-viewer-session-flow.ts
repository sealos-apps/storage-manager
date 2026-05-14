import type { ViewerAPI, ViewerSelection, ViewerSession, ViewerToken } from '@/features/viewer/types/viewer'

import { startTransition, useCallback, useEffect, useRef, useState } from 'react'

import { viewerApi } from '@/features/viewer/api/viewer-api'
import { normalizeViewerError } from '@/features/viewer/api/viewer-error'

type FlowStatus = 'idle' | 'creating' | 'polling' | 'issuing-token' | 'ready' | 'failed'

interface UseViewerSessionFlowInput {
	api?: ViewerAPI
	pollIntervalMs?: number
}

export interface ViewerSessionFlow {
	error: ReturnType<typeof normalizeViewerError> | null
	reset: () => void
	session: ViewerSession | null
	start: (pvc: ViewerSelection) => Promise<void>
	status: FlowStatus
	token: ViewerToken | null
}

function shouldPoll(session: ViewerSession) {
	return session.status === 'creating' || session.status === 'active'
}

export function useViewerSessionFlow({
	api = viewerApi,
	pollIntervalMs = 2_000,
}: UseViewerSessionFlowInput = {}): ViewerSessionFlow {
	const [error, setError] = useState<ReturnType<typeof normalizeViewerError> | null>(null)
	const [selectedPVC, setSelectedPVC] = useState<ViewerSelection | null>(null)
	const [session, setSession] = useState<ViewerSession | null>(null)
	const [status, setStatus] = useState<FlowStatus>('idle')
	const [token, setToken] = useState<ViewerToken | null>(null)
	const issuingTokenRef = useRef(false)

	const createForPVC = useCallback(async (pvc: ViewerSelection) => {
		setStatus('creating')
		setError(null)
		setToken(null)
		const nextSession = await api.createViewerSession({
			namespace: pvc.namespace,
			pvcName: pvc.pvcName,
		})
		setSession(nextSession)
		setStatus(nextSession.status === 'ready' ? 'issuing-token' : 'polling')
	}, [api])

	const start = useCallback(async (pvc: ViewerSelection) => {
		setSelectedPVC(pvc)
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
		setSelectedPVC(null)
		setSession(null)
		setStatus('idle')
		setToken(null)
		issuingTokenRef.current = false
	}, [])

	useEffect(() => {
		if (!session || !shouldPoll(session)) {
			return undefined
		}

		const id = window.setInterval(() => {
			void api.getViewerSession(session.id)
				.then((nextSession) => {
					setSession(nextSession)
					if (nextSession.status === 'ready') {
						setStatus('issuing-token')
					}
					else if (nextSession.status === 'failed' || nextSession.status === 'expired' || nextSession.status === 'closed') {
						setStatus('failed')
					}
				})
				.catch(async (caught) => {
					const nextError = normalizeViewerError(caught)
					if (nextError.code === 'VIEWER_SESSION_NOT_FOUND' && selectedPVC) {
						try {
							await createForPVC(selectedPVC)
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
	}, [api, createForPVC, pollIntervalMs, selectedPVC, session])

	useEffect(() => {
		if (!session || session.status !== 'ready' || !session.token_ready || issuingTokenRef.current || token) {
			return
		}
		issuingTokenRef.current = true
		startTransition(() => setStatus('issuing-token'))
		void api.issueViewerToken(session.id)
			.then((nextToken) => {
				setToken(nextToken)
				setStatus('ready')
			})
			.catch((caught) => {
				setError(normalizeViewerError(caught))
				setStatus('failed')
			})
			.finally(() => {
				issuingTokenRef.current = false
			})
	}, [api, session, token])

	return {
		error,
		reset,
		session,
		start,
		status,
		token,
	}
}
