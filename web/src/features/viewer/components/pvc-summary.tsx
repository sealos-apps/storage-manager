import type { ReactNode } from 'react'

import type { PVC } from '@/features/viewer/types/viewer'

import { Database, HardDrive, Server, ShieldAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'
import { formatBytes } from '@/features/viewer/utils/format-capacity'

interface PVCSummaryProps {
	pvcs: PVC[]
}

export function PVCSummary({ pvcs }: PVCSummaryProps) {
	const { t } = useTranslation()
	const mounted = pvcs.filter(pvc => pvc.mounted).length
	const supported = pvcs.filter(pvc => pvc.viewer_supported).length
	const unsupported = pvcs.length - supported
	const capacity = pvcs.reduce((total, pvc) => total + pvc.capacity_bytes, 0)

	return (
		<section className="grid gap-3 md:grid-cols-4">
			<SummaryTile icon={<Database />} label={t('viewer.summaryTotal')} value={String(pvcs.length)} />
			<SummaryTile icon={<Server />} label={t('viewer.summaryMounted')} value={String(mounted)} />
			<SummaryTile icon={<HardDrive />} label={t('viewer.capacity')} value={formatBytes(capacity)} />
			<SummaryTile icon={<ShieldAlert />} label={t('viewer.summaryUnsupported')} value={String(unsupported)} />
		</section>
	)
}

interface SummaryTileProps {
	icon: ReactNode
	label: string
	value: string
}

function SummaryTile({ icon, label, value }: SummaryTileProps) {
	return (
		<Card className="rounded-lg py-4">
			<CardContent className="flex items-center gap-3 px-4">
				<div className="flex size-9 items-center justify-center rounded-md border bg-muted text-muted-foreground [&_svg]:size-4">
					{icon}
				</div>
				<div className="min-w-0">
					<div className="text-sm text-muted-foreground">{label}</div>
					<div className="mt-1 text-xl font-semibold">{value}</div>
				</div>
			</CardContent>
		</Card>
	)
}
