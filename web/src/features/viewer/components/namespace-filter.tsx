import type { AdminNamespace } from '@/features/viewer/types/viewer'

import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
	Combobox,
	ComboboxContent,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
} from '@/components/ui/combobox'

interface NamespaceFilterProps {
	canSelectNamespaces: boolean
	isLoadingNamespaces?: boolean
	namespace: string
	namespaces: AdminNamespace[]
	onNamespaceChange: (namespace: string) => void
}

export function NamespaceFilter({
	canSelectNamespaces,
	isLoadingNamespaces = false,
	namespace,
	namespaces,
	onNamespaceChange,
}: NamespaceFilterProps) {
	const { t } = useTranslation()
	const namespaceNames = useMemo(() => namespaces.map(item => item.name), [namespaces])
	const currentContextNames = useMemo(
		() => new Set(namespaces.filter(item => item.is_current_context).map(item => item.name)),
		[namespaces],
	)
	const [value, setValue] = useState<string | null>(null)
	const selectedNamespace = namespace || value || ''

	return (
		<div className="flex w-full flex-col gap-2 md:w-auto md:flex-row md:items-center md:justify-end">
			{canSelectNamespaces
				? (
						<Combobox
							onValueChange={(nextNamespace) => {
								if (nextNamespace) {
									setValue(nextNamespace)
									onNamespaceChange(nextNamespace)
								}
							}}
							value={selectedNamespace}
						>
							<ComboboxInput
								aria-label={t('viewer.systemNamespace')}
								className="w-full md:w-60"
								disabled={isLoadingNamespaces || namespaces.length === 0}
								onFocus={event => event.currentTarget.select()}
								placeholder={isLoadingNamespaces ? t('common.loading') : t('viewer.filterNamespaces')}
							/>
							<ComboboxContent align="end">
								<ComboboxList>
									{namespaceNames.map(item => (
										<ComboboxItem index={namespaceNames.indexOf(item)} key={item} value={item}>
											<span className="min-w-0 flex-1 truncate">{item}</span>
											{currentContextNames.has(item) ? <span className="text-xs text-muted-foreground">{t('common.current')}</span> : null}
										</ComboboxItem>
									))}
								</ComboboxList>
							</ComboboxContent>
						</Combobox>
					)
				: (
						<div className="flex h-9 w-full items-center rounded-md border bg-muted px-3 text-sm md:w-48">
							<span className="truncate text-muted-foreground">
								{t('common.namespace')}
								:
								{' '}
							</span>
							<span className="truncate font-medium">{namespace || t('common.loading')}</span>
						</div>
					)}
		</div>
	)
}
