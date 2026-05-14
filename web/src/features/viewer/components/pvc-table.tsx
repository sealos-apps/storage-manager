import type { PVC } from '@/features/viewer/types/viewer'

import { ExternalLink, Server } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { PVCStatusBadge } from '@/features/viewer/components/pvc-status-badge'
import { useViewerSearch } from '@/features/viewer/stores/viewer-ui-store'
import { formatBytes } from '@/features/viewer/utils/format-capacity'
import { canLaunchViewer, launchBlockReason } from '@/features/viewer/utils/viewer-status'

interface PVCTableProps {
	onLaunch: (pvc: PVC) => void
	pvcs: PVC[]
}

export function PVCTable({ onLaunch, pvcs }: PVCTableProps) {
	const search = useViewerSearch()
	const { t } = useTranslation()
	const filtered = useMemo(() => {
		const keyword = search.trim().toLowerCase()
		if (!keyword) {
			return pvcs
		}
		return pvcs.filter((pvc) => {
			const mountedPodNames = pvc.mounted_pods.map(pod => pod.name).join(' ')
			return `${pvc.namespace} ${pvc.name} ${mountedPodNames}`.toLowerCase().includes(keyword)
		})
	}, [pvcs, search])

	return (
		<Card className="min-h-0 rounded-lg">
			<CardHeader className="gap-2">
				<CardTitle>{t('viewer.pvcList')}</CardTitle>
				<CardDescription>
					{t('app.subtitle')}
				</CardDescription>
			</CardHeader>
			<CardContent>
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>{t('viewer.pvc')}</TableHead>
							<TableHead>{t('viewer.capacity')}</TableHead>
							<TableHead>{t('viewer.accessModes')}</TableHead>
							<TableHead>{t('viewer.mountedPods')}</TableHead>
							<TableHead>{t('viewer.viewerMode')}</TableHead>
							<TableHead className="text-right">{t('actions.launchViewer')}</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{filtered.map(pvc => (
							<TableRow key={pvc.uid}>
								<TableCell>
									<div className="flex items-center gap-3">
										<div className="flex size-8 items-center justify-center rounded-md border bg-muted text-muted-foreground">
											<Server className="size-4" />
										</div>
										<div className="min-w-0">
											<div className="font-medium">{pvc.name}</div>
											<div className="text-xs text-muted-foreground">{pvc.namespace}</div>
										</div>
									</div>
								</TableCell>
								<TableCell>{pvc.capacity || formatBytes(pvc.capacity_bytes)}</TableCell>
								<TableCell>
									<div className="flex flex-wrap gap-1">
										{pvc.access_modes.map(mode => (
											<span className="rounded-md bg-muted px-2 py-1 text-xs" key={mode}>
												{mode}
											</span>
										))}
									</div>
								</TableCell>
								<TableCell>
									{pvc.mounted_pods.length > 0
										? (
												<div className="flex flex-col gap-1 text-xs">
													{pvc.mounted_pods.slice(0, 2).map(pod => (
														<div key={`${pod.namespace}/${pod.name}`}>
															{pod.name}
															{pod.node_name ? ` · ${pod.node_name}` : ''}
														</div>
													))}
												</div>
											)
										: <span className="text-muted-foreground">{t('viewer.noMountedPods')}</span>}
								</TableCell>
								<TableCell>
									<PVCStatusBadge pvc={pvc} />
									{pvc.viewer_scheduling.requires_node
										? (
												<div className="mt-1 text-xs text-muted-foreground">
													{t('viewer.scheduling')}
													{': '}
													{pvc.viewer_scheduling.node_name}
												</div>
											)
										: null}
								</TableCell>
								<TableCell className="text-right">
									<LaunchButton
										disabled={!canLaunchViewer(pvc)}
										onClick={() => onLaunch(pvc)}
										reason={launchBlockReason(pvc)}
									/>
								</TableCell>
							</TableRow>
						))}
						{filtered.length === 0
							? (
									<TableRow>
										<TableCell className="py-12 text-center text-muted-foreground" colSpan={6}>
											{t('common.empty')}
										</TableCell>
									</TableRow>
								)
							: null}
					</TableBody>
				</Table>
			</CardContent>
		</Card>
	)
}

interface LaunchButtonProps {
	disabled: boolean
	onClick: () => void
	reason: string
}

function LaunchButton({ disabled, onClick, reason }: LaunchButtonProps) {
	const { t } = useTranslation()
	const button = (
		<Button disabled={disabled} onClick={onClick} size="sm">
			<ExternalLink />
			{t('actions.launchViewer')}
		</Button>
	)

	if (!disabled) {
		return button
	}

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span>{button}</span>
			</TooltipTrigger>
			<TooltipContent>{reason || t('status.unsupported')}</TooltipContent>
		</Tooltip>
	)
}
