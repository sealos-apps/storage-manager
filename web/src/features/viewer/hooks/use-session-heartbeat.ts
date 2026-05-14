import type { ViewerAPI } from '@/features/viewer/types/viewer'

import { useEffect } from 'react'

import { viewerApi } from '@/features/viewer/api/viewer-api'

interface UseSessionHeartbeatInput {
	api?: ViewerAPI
	enabled: boolean
	intervalMs?: number
	viewerSessionID: string | null
}

export function useSessionHeartbeat({
	api = viewerApi,
	enabled,
	intervalMs = 20_000,
	viewerSessionID,
}: UseSessionHeartbeatInput) {
	useEffect(() => {
		if (!enabled || !viewerSessionID) {
			return undefined
		}

		let cancelled = false
		const sendHeartbeat = () => {
			void api.heartbeatViewerSession(viewerSessionID).catch(() => {
				void cancelled
			})
		}

		sendHeartbeat()
		const id = window.setInterval(sendHeartbeat, intervalMs)
		return () => {
			cancelled = true
			window.clearInterval(id)
		}
	}, [api, enabled, intervalMs, viewerSessionID])
}
