import { Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useViewerNamespace, useViewerSearch, viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'

interface NamespaceFilterProps {
	namespaces: string[]
}

export function NamespaceFilter({ namespaces }: NamespaceFilterProps) {
	const namespace = useViewerNamespace()
	const search = useViewerSearch()
	const { t } = useTranslation()

	return (
		<div className="flex w-full flex-col gap-2 md:flex-row md:items-center md:justify-end">
			<div className="relative w-full md:w-80">
				<Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
				<Input
					aria-label={t('common.search')}
					className="pl-9"
					onChange={event => viewerUIStore.actions.setSearch(event.target.value)}
					placeholder={t('viewer.searchPlaceholder')}
					value={search}
				/>
			</div>
			<Select
				onValueChange={value => viewerUIStore.actions.setNamespace(value)}
				value={namespace}
			>
				<SelectTrigger aria-label={t('common.namespace')} className="w-full md:w-48">
					<SelectValue />
				</SelectTrigger>
				<SelectContent>
					{namespaces.map(item => (
						<SelectItem key={item} value={item}>
							{item}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
	)
}
