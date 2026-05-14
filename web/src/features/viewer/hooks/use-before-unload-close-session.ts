import type { ViewerAPI } from '@/features/viewer/types/viewer'

import { useEffect } from 'react'

import { viewerApi } from '@/features/viewer/api/viewer-api'

interface UseBeforeUnloadCloseSessionInput {
	api?: ViewerAPI
	enabled: boolean
	viewerSessionID: string | null
}

export function useBeforeUnloadCloseSession({
	api = viewerApi,
	enabled,
	viewerSessionID,
}: UseBeforeUnloadCloseSessionInput) {
	useEffect(() => {
		if (!enabled || !viewerSessionID) {
			return undefined
		}

		const closeSession = () => {
			void api.closeViewerSession(viewerSessionID).catch(() => undefined)
		}

		window.addEventListener('pagehide', closeSession)
		return () => window.removeEventListener('pagehide', closeSession)
	}, [api, enabled, viewerSessionID])
}
