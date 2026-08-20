import type { RecycleEntry } from '@/features/file-manager/types/file-manager'

import { Loader2, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import { remainingTrashDays } from '@/features/file-manager/utils/file-tree'
import { formatBytes } from '@/features/viewer/utils/format-capacity'

interface RecycleBinTableProps {
	isLoading: boolean
	items: RecycleEntry[]
	onRestore: (entry: RecycleEntry) => void
	sessionReady: boolean
}

export function RecycleBinTable({ isLoading, items, onRestore, sessionReady }: RecycleBinTableProps) {
	const { t } = useTranslation()

	return (
		<div className="rounded-lg border bg-card">
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>{t('trash.columns.name')}</TableHead>
						<TableHead>{t('trash.columns.originalPath')}</TableHead>
						<TableHead>{t('trash.columns.size')}</TableHead>
						<TableHead>{t('trash.columns.deletedAt')}</TableHead>
						<TableHead>{t('trash.columns.remainingDays')}</TableHead>
						<TableHead className="text-right">{t('files.columns.actions')}</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{!sessionReady || isLoading
						? (
								<TableRow>
									<TableCell className="py-12 text-center text-muted-foreground" colSpan={6}>
										<Loader2 aria-label={t('common.loading')} className="mx-auto size-5 animate-spin" />
									</TableCell>
								</TableRow>
							)
						: null}
					{sessionReady && !isLoading
						? items.map(item => (
								<TableRow key={item.id}>
									<TableCell className="font-medium">{item.name}</TableCell>
									<TableCell className="font-mono text-xs text-muted-foreground">{item.originalPath}</TableCell>
									<TableCell>{formatBytes(item.size)}</TableCell>
									<TableCell>{item.deletedAt}</TableCell>
									<TableCell>{remainingTrashDays(item.deletedAt)}</TableCell>
									<TableCell className="text-right">
										<Button onClick={() => onRestore(item)} size="sm" variant="outline">
											<RotateCcw data-icon="inline-start" />
											{t('trash.restore')}
										</Button>
									</TableCell>
								</TableRow>
							))
						: null}
					{sessionReady && !isLoading && items.length === 0
						? (
								<TableRow>
									<TableCell className="py-12 text-center text-muted-foreground" colSpan={6}>
										{t('trash.empty')}
									</TableCell>
								</TableRow>
							)
						: null}
				</TableBody>
			</Table>
		</div>
	)
}
