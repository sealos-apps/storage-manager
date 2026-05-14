import type { ViewerSession } from '@/features/viewer/types/viewer'

import { Clock3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { ViewerSessionStatusBadge } from '@/features/viewer/components/pvc-status-badge'

interface SessionLifecycleBannerProps {
	session: ViewerSession | null
}

export function SessionLifecycleBanner({ session }: SessionLifecycleBannerProps) {
	const { t } = useTranslation()

	if (!session) {
		return (
			<Alert>
				<Clock3 />
				<AlertTitle>{t('viewer.sessionLifecycle')}</AlertTitle>
				<AlertDescription>{t('viewer.noSelection')}</AlertDescription>
			</Alert>
		)
	}

	return (
		<Alert>
			<Clock3 />
			<AlertTitle className="flex items-center gap-2">
				{t('viewer.activeSession')}
				<ViewerSessionStatusBadge session={session} />
			</AlertTitle>
			<AlertDescription>
				{t('viewer.podSession')}
				{': '}
				{session.pod_session_id}
				{' · '}
				{t('viewer.lastHeartbeat')}
				{': '}
				{session.last_heartbeat_at || '-'}
			</AlertDescription>
		</Alert>
	)
}
