import type { UseQueryResult } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import type { PVC, StorageClass } from '@/features/viewer/types/viewer'

import { CircleAlert, FolderOpen, HardDrive, MoreHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Progress } from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { translateViewerError } from '@/features/viewer/api/viewer-error'
import { ErrorCallout } from '@/features/viewer/components/error-callout'
import { PVCListSkeleton } from '@/features/viewer/components/loading-skeletons'
import { PVCStatusBadge } from '@/features/viewer/components/pvc-status-badge'
import { useViewerSearch } from '@/features/viewer/stores/viewer-ui-store'
import { formatQuantity, quantityPercent, sumQuantities } from '@/features/viewer/utils/storage-quantity'
import { canLaunchViewer } from '@/features/viewer/utils/viewer-status'

interface VolumesViewProps {
	actions: ReactNode
	fileManagementEnabled: boolean
	showNamespaceColumn?: boolean
	onDelete: (pvc: PVC) => void
	onDescribe: (pvc: PVC) => void
	onEditYAML: (pvc: PVC) => void
	onExpand: (pvc: PVC) => void
	onOpenFiles: (pvc: PVC) => void
	pvcQuery: UseQueryResult<PVC[], Error>
	pvcs: PVC[]
	storageClasses: StorageClass[]
}

export function VolumesView({
	actions,
	fileManagementEnabled,
	showNamespaceColumn = false,
	onDelete,
	onDescribe,
	onEditYAML,
	onExpand,
	onOpenFiles,
	pvcQuery,
	pvcs,
	storageClasses,
}: VolumesViewProps) {
	const { t } = useTranslation()
	const search = useViewerSearch().trim().toLowerCase()
	const filteredPVCs = search
		? pvcs.filter((pvc) => {
				const mountedPodNames = pvc.mounted_pods.map(pod => pod.name).join(' ')
				return `${pvc.namespace} ${pvc.name} ${pvc.storage_class_name} ${mountedPodNames}`.toLowerCase().includes(search)
			})
		: pvcs
	const capacity = sumQuantities(pvcs.map(pvc => pvc.capacity))
	const mountDetectionAvailable = pvcs.every(pvc => pvc.mount_status !== 'unknown')
	const mounted = mountDetectionAvailable ? pvcs.filter(pvc => pvc.mounted).length : null
	const unused = mounted === null ? null : pvcs.length - mounted

	return (
		<section className="flex flex-col gap-4">
			<header className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
				<div>
					<h2 className="text-xl font-semibold">{t('nav.volumes')}</h2>
					<p className="text-sm text-muted-foreground">{t('viewer.pvcListDescription')}</p>
				</div>
				{actions}
			</header>
			<Separator />

			<div className="grid gap-3 md:grid-cols-3">
				<MetricCard label={t('volumes.totalAllocated')} value={formatQuantity(capacity)} />
				<MetricCard label={t('volumes.mountedCount')} value={mounted === null ? t('volumes.mountDetectionUnavailable') : String(mounted)} />
				<MetricCard label={t('volumes.unusedCount')} value={unused === null ? t('volumes.mountDetectionUnavailable') : String(unused)} />
			</div>

			{pvcQuery.isLoading ? <PVCListSkeleton /> : null}
			{pvcQuery.error
				? (
						<ErrorCallout title={t('common.error')}>
							{translateViewerError(pvcQuery.error, t)}
						</ErrorCallout>
					)
				: null}
			{!pvcQuery.isLoading && !pvcQuery.error
				? (
						<div className="rounded-lg border bg-card">
							<Table>
								<TableHeader>
									<TableRow>
										<TableHead>{t('viewer.pvc')}</TableHead>
										{showNamespaceColumn ? <TableHead>{t('common.namespace')}</TableHead> : null}
										<TableHead>{t('volumes.usage')}</TableHead>
										<TableHead>{t('viewer.storageClass')}</TableHead>
										<TableHead>{t('viewer.accessModes')}</TableHead>
										<TableHead className="text-right">{t('files.columns.actions')}</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{filteredPVCs.map(pvc => (
										<PVCRow
											key={pvc.uid}
											onDelete={onDelete}
											onDescribe={onDescribe}
											onEditYAML={onEditYAML}
											onExpand={onExpand}
											onOpenFiles={onOpenFiles}
											fileManagementEnabled={fileManagementEnabled}
											pvc={pvc}
											showNamespaceColumn={showNamespaceColumn}
										/>
									))}
									{filteredPVCs.length === 0
										? (
												<TableRow>
													<TableCell className="py-12 text-center text-muted-foreground" colSpan={showNamespaceColumn ? 6 : 5}>
														{storageClasses.length === 0 ? t('common.empty') : t('volumes.empty')}
													</TableCell>
												</TableRow>
											)
										: null}
								</TableBody>
							</Table>
						</div>
					)
				: null}
		</section>
	)
}

function MetricCard({ label, value }: { label: string, value: string }) {
	return (
		<div className="rounded-lg border bg-card p-4">
			<div className="text-sm text-muted-foreground">{label}</div>
			<div className="mt-2 text-2xl font-semibold">{value}</div>
		</div>
	)
}

interface PVCRowProps {
	fileManagementEnabled: boolean
	onDelete: (pvc: PVC) => void
	onDescribe: (pvc: PVC) => void
	onEditYAML: (pvc: PVC) => void
	onExpand: (pvc: PVC) => void
	onOpenFiles: (pvc: PVC) => void
	pvc: PVC
	showNamespaceColumn: boolean
}

function PVCRow({ fileManagementEnabled, onDelete, onDescribe, onEditYAML, onExpand, onOpenFiles, pvc, showNamespaceColumn }: PVCRowProps) {
	const { t } = useTranslation()
	const mountedTarget = pvc.mounted_pods[0]
	const canDelete = !pvc.mounted

	return (
		<TableRow>
			<TableCell className="max-w-[28rem] whitespace-nowrap">
				<div className="flex min-w-0 items-center gap-3">
					<div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-muted text-muted-foreground">
						<HardDrive />
					</div>
					<div className="flex min-w-0 items-center gap-2">
						<span className="truncate font-medium">{pvc.name}</span>
						<div className="flex shrink-0 items-center gap-1">
							<PVCStatusBadge pvc={pvc} />
							<PVCMountedBadge mounted={pvc.mounted} mountedPodName={mountedTarget?.name} mountStatus={pvc.mount_status} />
						</div>
					</div>
				</div>
			</TableCell>
			{showNamespaceColumn
				? (
						<TableCell>
							<span className="text-sm">{pvc.namespace}</span>
						</TableCell>
					)
				: null}
			<TableCell>
				<PVCUsageCell pvc={pvc} />
			</TableCell>
			<TableCell>
				<span className="text-sm">{pvc.storage_class_name || '-'}</span>
			</TableCell>
			<TableCell>
				<div className="flex flex-wrap gap-1">
					{pvc.access_modes.map(mode => (
						<Badge key={mode} variant="outline">{mode}</Badge>
					))}
				</div>
			</TableCell>
			<TableCell>
				<div className="flex justify-end items-center gap-2">
					{fileManagementEnabled
						? (
								<Button
									disabled={!canLaunchViewer(pvc)}
									onClick={() => onOpenFiles(pvc)}
									size="sm"
								>
									<FolderOpen data-icon="inline-start" />
									{t('files.browse')}
								</Button>
							)
						: null}
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button aria-label={t('actions.more')} size="icon" variant="outline">
								<MoreHorizontal />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuGroup>
								<DropdownMenuItem onSelect={() => onExpand(pvc)}>
									{t('volumes.expand')}
								</DropdownMenuItem>
								<DropdownMenuItem onSelect={() => onDescribe(pvc)}>
									{t('storageClasses.describe')}
								</DropdownMenuItem>
								<DropdownMenuItem onSelect={() => onEditYAML(pvc)}>
									{t('storageClasses.yaml')}
								</DropdownMenuItem>
								<DropdownMenuItem
									disabled={!canDelete}
									onSelect={() => onDelete(pvc)}
									variant="destructive"
								>
									{t('actions.delete')}
								</DropdownMenuItem>
							</DropdownMenuGroup>
						</DropdownMenuContent>
					</DropdownMenu>
				</div>
			</TableCell>
		</TableRow>
	)
}

function PVCMountedBadge({
	mounted,
	mountedPodName,
	mountStatus,
}: {
	mounted: boolean
	mountedPodName?: string
	mountStatus?: string
}) {
	const { t } = useTranslation()
	if (mountStatus === 'unknown') {
		return <Badge variant="outline">{t('volumes.mountDetectionUnavailable')}</Badge>
	}
	const badge = (
		<Badge variant={mounted ? 'default' : 'outline'}>
			{mounted ? t('status.mounted') : t('status.ready')}
		</Badge>
	)
	if (!mounted || !mountedPodName) {
		return badge
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					className="h-auto rounded-full px-2 py-0.5 text-xs"
					size="sm"
					title={mountedPodName}
					variant="default"
				>
					{t('status.mounted')}
				</Button>
			</TooltipTrigger>
			<TooltipContent>{mountedPodName}</TooltipContent>
		</Tooltip>
	)
}

function PVCUsageCell({ pvc }: { pvc: PVC }) {
	const { t } = useTranslation()
	const stats = pvc.volume_stats
	if (!stats) {
		return (
			<div className="grid min-h-10 min-w-40 content-center">
				<span className="text-sm text-muted-foreground">{t('volumes.usageUnavailable')}</span>
			</div>
		)
	}
	const metricCapacity = stats.metricCapacity
	const percent = quantityPercent(stats.used, metricCapacity)
	const mismatch = stats.status !== 'ready'
	const freeText = t('volumes.usageFree', { size: formatQuantity(stats.available) })

	return (
		<div className="grid min-h-10 min-w-40 content-center gap-1.5">
			<div className="flex items-center justify-between gap-2 text-sm">
				<div className="flex min-w-0 items-center gap-1.5">
					<span className="truncate font-medium">{`${formatQuantity(stats.used)} / ${formatQuantity(metricCapacity)}`}</span>
					{mismatch
						? (
								<Tooltip>
									<TooltipTrigger asChild>
										<Button
											aria-label={t('volumes.usageMismatchLabel', { pvc: pvc.name })}
											className="size-5 shrink-0 text-amber-700"
											size="icon"
											variant="ghost"
										>
											<CircleAlert />
										</Button>
									</TooltipTrigger>
									<TooltipContent>
										<div className="grid gap-1">
											<div>{t('volumes.usageMismatch')}</div>
											<div>{t('volumes.usageRequestedCapacity', { size: formatQuantity(pvc.capacity) })}</div>
										</div>
									</TooltipContent>
								</Tooltip>
							)
						: null}
				</div>
				<span className="text-xs text-muted-foreground">{`${percent}%`}</span>
			</div>
			<Tooltip>
				<TooltipTrigger asChild>
					<Progress
						aria-label={t('volumes.usageProgressLabel', { pvc: pvc.name })}
						title={freeText}
						value={percent}
					/>
				</TooltipTrigger>
				<TooltipContent>{freeText}</TooltipContent>
			</Tooltip>
		</div>
	)
}
