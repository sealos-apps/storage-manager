import type { UseMutationResult, UseQueryResult } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import type { StorageClass } from '@/features/viewer/types/viewer'

import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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

interface StorageClassAdminViewProps {
	actions: ReactNode
	deleteMutation: UseMutationResult<StorageClass, Error, string>
	onDelete: (name: string) => void
	onDescribe: (name: string) => void
	onEditMetadata: (name: string) => void
	onEditYAML: (name: string) => void
	query: UseQueryResult<StorageClass[], Error>
}

export function StorageClassAdminView({
	actions,
	onDelete,
	onDescribe,
	onEditMetadata,
	onEditYAML,
	query,
}: StorageClassAdminViewProps) {
	const { i18n, t } = useTranslation()
	const items = query.data ?? []

	return (
		<section className="flex flex-col gap-4">
			<header className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
				<div>
					<h2 className="text-xl font-semibold">{t('storageClasses.title')}</h2>
					<p className="text-sm text-muted-foreground">{t('storageClasses.description')}</p>
				</div>
				{actions}
			</header>
			<Separator />
			{query.error
				? (
						<ErrorCallout title={t('common.error')}>
							{translateViewerError(query.error, t)}
						</ErrorCallout>
					)
				: null}
			<div className="overflow-x-auto rounded-lg border bg-card">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>{t('storageClasses.name')}</TableHead>
							<TableHead>{t('storageClasses.displayName')}</TableHead>
							<TableHead>{t('storageClasses.provisioner')}</TableHead>
							<TableHead>{t('storageClasses.reclaimPolicy')}</TableHead>
							<TableHead>{t('storageClasses.volumeBindingMode')}</TableHead>
							<TableHead>{t('storageClasses.allowVolumeExpansion')}</TableHead>
							<TableHead>{t('storageClasses.pvcUsage')}</TableHead>
							<TableHead className="text-right">{t('files.columns.actions')}</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{items.map(storageClass => (
							<TableRow key={storageClass.name}>
								<TableCell>
									<div className="flex items-center gap-1">
										<div className="font-medium">{storageClass.name}</div>
										{storageClass.is_default ? <Badge variant="secondary">{t('common.default')}</Badge> : null}
									</div>
								</TableCell>
								<TableCell>
									<StorageClassDisplayName storageClass={storageClass} locale={i18n.language} />
								</TableCell>
								<TableCell>{storageClass.provisioner}</TableCell>
								<TableCell>{storageClass.reclaim_policy || '-'}</TableCell>
								<TableCell>{storageClass.volume_binding_mode || '-'}</TableCell>
								<TableCell>{storageClass.allow_volume_expansion ? t('common.yes') : t('common.no')}</TableCell>
								<TableCell>{storageClass.in_use_pvc_count}</TableCell>
								<TableCell>
									<div className="flex justify-end gap-2">
										<Button onClick={() => onDescribe(storageClass.name)} size="sm" type="button" variant="outline">
											{t('storageClasses.describe')}
										</Button>
										<Button onClick={() => onEditYAML(storageClass.name)} size="sm" type="button" variant="outline">
											{t('storageClasses.yaml')}
										</Button>
										<Button onClick={() => onEditMetadata(storageClass.name)} size="sm" type="button" variant="outline">
											{t('actions.edit')}
										</Button>
										<DeleteButton
											onDelete={() => onDelete(storageClass.name)}
											storageClass={storageClass}
										/>
									</div>
								</TableCell>
							</TableRow>
						))}
						{items.length === 0
							? (
									<TableRow>
										<TableCell className="py-12 text-center text-muted-foreground" colSpan={8}>
											{query.isLoading ? t('common.loading') : t('common.empty')}
										</TableCell>
									</TableRow>
								)
							: null}
					</TableBody>
				</Table>
			</div>
		</section>
	)
}

function StorageClassDisplayName({ locale, storageClass }: { locale: string, storageClass: StorageClass }) {
	const names = storageClass.display_names ?? {}
	const display = names[locale] ?? names[locale.split('-')[0] ?? ''] ?? storageClass.name
	const entries = Object.entries(names).sort(([left], [right]) => left.localeCompare(right))
	const text = <span className="text-sm">{display}</span>
	if (entries.length === 0) {
		return text
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="inline-flex cursor-default">{text}</span>
			</TooltipTrigger>
			<TooltipContent>
				<div className="grid gap-1">
					{entries.map(([key, value]) => <div key={key}>{`${key}: ${value}`}</div>)}
				</div>
			</TooltipContent>
		</Tooltip>
	)
}

function DeleteButton({
	onDelete,
	storageClass,
}: {
	onDelete: () => void
	storageClass: StorageClass
}) {
	const { t } = useTranslation()
	const reason = storageClassDeleteBlockMessage(storageClass, t)
	const button = (
		<Button
			disabled={Boolean(reason)}
			onClick={onDelete}
			size="sm"
			type="button"
			variant="destructive"
		>
			{t('actions.delete')}
		</Button>
	)
	if (!reason) {
		return button
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span>{button}</span>
			</TooltipTrigger>
			<TooltipContent>{reason}</TooltipContent>
		</Tooltip>
	)
}

function storageClassDeleteBlockMessage(storageClass: StorageClass, t: ReturnType<typeof useTranslation>['t']) {
	if (!storageClass.managed_by_storage_manager || storageClass.delete_blocked_reason === 'not_managed') {
		return t('storageClasses.deleteBlockedNotManaged')
	}
	if (storageClass.in_use_pvc_count > 0 || storageClass.delete_blocked_reason === 'in_use') {
		return t('storageClasses.deleteBlockedInUse', { count: storageClass.in_use_pvc_count })
	}
	return ''
}
