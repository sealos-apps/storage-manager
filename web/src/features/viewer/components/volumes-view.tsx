import type { UseQueryResult } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import type { PVC, StorageClass } from '@/features/viewer/types/viewer'

import { FolderOpen, HardDrive, MoreHorizontal } from 'lucide-react'
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
import { Separator } from '@/components/ui/separator'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import { translateViewerError } from '@/features/viewer/api/viewer-error'
import { ErrorCallout } from '@/features/viewer/components/error-callout'
import { PVCListSkeleton } from '@/features/viewer/components/loading-skeletons'
import { PVCStatusBadge } from '@/features/viewer/components/pvc-status-badge'
import { useViewerSearch } from '@/features/viewer/stores/viewer-ui-store'
import { formatBytes } from '@/features/viewer/utils/format-capacity'
import { canLaunchViewer } from '@/features/viewer/utils/viewer-status'

interface VolumesViewProps {
	actions: ReactNode
	fileManagementEnabled: boolean
	onDelete: (pvc: PVC) => void
	onExpand: (pvc: PVC) => void
	onOpenFiles: (pvc: PVC) => void
	pvcQuery: UseQueryResult<PVC[], Error>
	pvcs: PVC[]
	storageClasses: StorageClass[]
}

export function VolumesView({
	actions,
	fileManagementEnabled,
	onDelete,
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
	const capacity = pvcs.reduce((total, pvc) => total + pvc.capacity_bytes, 0)
	const mounted = pvcs.filter(pvc => pvc.mounted).length
	const unused = pvcs.length - mounted

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
				<MetricCard label={t('volumes.totalAllocated')} value={formatBytes(capacity)} />
				<MetricCard label={t('volumes.mountedCount')} value={String(mounted)} />
				<MetricCard label={t('volumes.unusedCount')} value={String(unused)} />
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
										<TableHead>{t('status.label')}</TableHead>
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
											onExpand={onExpand}
											onOpenFiles={onOpenFiles}
											fileManagementEnabled={fileManagementEnabled}
											pvc={pvc}
										/>
									))}
									{filteredPVCs.length === 0
										? (
												<TableRow>
													<TableCell className="py-12 text-center text-muted-foreground" colSpan={5}>
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
	onExpand: (pvc: PVC) => void
	onOpenFiles: (pvc: PVC) => void
	pvc: PVC
}

function PVCRow({ fileManagementEnabled, onDelete, onExpand, onOpenFiles, pvc }: PVCRowProps) {
	const { t } = useTranslation()
	const mountedTarget = pvc.mounted_pods[0]
	const canDelete = !pvc.mounted

	return (
		<TableRow>
			<TableCell>
				<div className="flex min-w-0 items-center gap-3">
					<div className="flex size-9 items-center justify-center rounded-md border bg-muted text-muted-foreground">
						<HardDrive />
					</div>
					<div className="min-w-0">
						<div className="truncate font-medium">{pvc.name}</div>
						<div className="truncate text-xs text-muted-foreground">
							{mountedTarget ? `${mountedTarget.name} · ${mountedTarget.namespace}` : pvc.namespace}
						</div>
					</div>
				</div>
			</TableCell>
			<TableCell>
				<div className="flex items-center gap-1">
					<PVCStatusBadge pvc={pvc} />
					<span className="text-xs text-muted-foreground">
						{pvc.mounted ? t('status.mounted') : t('status.ready')}
					</span>
				</div>
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
