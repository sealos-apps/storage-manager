import type { PVC, ViewerSession } from '@/features/viewer/types/viewer'

import { CircleAlert, CircleCheck, Clock, Lock, Pencil } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

interface PVCStatusBadgeProps {
	pvc: PVC
}

interface ViewerSessionStatusBadgeProps {
	session: ViewerSession | null
}

export function PVCStatusBadge({ pvc }: PVCStatusBadgeProps) {
	const { t } = useTranslation()

	if (!pvc.viewer_supported) {
		return (
			<Badge className="gap-1" variant="destructive">
				<CircleAlert />
				{t('status.unsupported')}
			</Badge>
		)
	}

	if (pvc.viewer_mode === 'readonly') {
		return (
			<Badge className="gap-1" variant="outline">
				<Lock />
				{t('status.readOnly')}
			</Badge>
		)
	}

	return (
		<Badge className="gap-1" variant="secondary">
			<Pencil />
			{t('status.readWrite')}
		</Badge>
	)
}

export function ViewerSessionStatusBadge({ session }: ViewerSessionStatusBadgeProps) {
	const { t } = useTranslation()
	const status = session?.status ?? 'idle'
	const ready = status === 'ready'
	const failed = status === 'failed' || status === 'expired' || status === 'closed'

	return (
		<Badge
			className="gap-1"
			variant={failed ? 'destructive' : ready ? 'default' : 'secondary'}
		>
			{ready ? <CircleCheck /> : failed ? <CircleAlert /> : <Clock />}
			{t(`status.${status}`, { defaultValue: status })}
		</Badge>
	)
}
