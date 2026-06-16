import type { AdminNamespace } from '@/features/viewer/types/viewer'

import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
	Combobox,
	ComboboxCollection,
	ComboboxContent,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
} from '@/components/ui/combobox'
import { ALL_NAMESPACES } from '@/features/viewer/api/viewer-constants'

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
	const options = useMemo(
		() => canSelectNamespaces ? [ALL_NAMESPACES, ...namespaceNames] : namespaceNames,
		[canSelectNamespaces, namespaceNames],
	)
	const currentContextNames = useMemo(
		() => new Set(namespaces.filter(item => item.is_current_context).map(item => item.name)),
		[namespaces],
	)
	const [value, setValue] = useState<string | null>(null)
	const selectedNamespace = namespace || value || ''
	const optionLabel = useCallback((item: string) => item === ALL_NAMESPACES ? t('viewer.allNamespaces') : item, [t])
	const selectedLabel = selectedNamespace ? optionLabel(selectedNamespace) : ''
	const [isSearching, setIsSearching] = useState(false)
	const [searchValue, setSearchValue] = useState('')
	const inputValue = isSearching ? searchValue : selectedLabel
	const filterOption = useCallback((item: string, query: string, itemToString?: (item: string) => string) => {
		if (item === ALL_NAMESPACES) {
			return true
		}

		const normalizedQuery = query.trim().toLowerCase()
		const label = itemToString ? itemToString(item) : optionLabel(item)
		return normalizedQuery === '' || label.toLowerCase().includes(normalizedQuery)
	}, [optionLabel])

	return (
		<div className="flex w-full flex-col gap-2 md:w-auto md:flex-row md:items-center md:justify-end">
			{canSelectNamespaces
				? (
						<Combobox
							filter={filterOption}
							inputValue={inputValue}
							items={options}
							itemToStringLabel={optionLabel}
							onInputValueChange={setSearchValue}
							onValueChange={(nextNamespace) => {
								if (nextNamespace) {
									setIsSearching(false)
									setSearchValue('')
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
								onBlur={() => {
									setIsSearching(false)
									setSearchValue('')
								}}
								onFocus={() => {
									setIsSearching(true)
									setSearchValue('')
								}}
								placeholder={isLoadingNamespaces ? t('common.loading') : t('viewer.filterNamespaces')}
							/>
							<ComboboxContent align="end">
								<NamespaceOptionList currentContextNames={currentContextNames} optionLabel={optionLabel} />
							</ComboboxContent>
						</Combobox>
					)
				: (
						<div className="flex h-9 w-full items-center text-sm md:w-48">
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

interface NamespaceOptionListProps {
	currentContextNames: Set<string>
	optionLabel: (item: string) => string
}

function NamespaceOptionList({ currentContextNames, optionLabel }: NamespaceOptionListProps) {
	const { t } = useTranslation()

	return (
		<ComboboxList>
			<ComboboxCollection>
				{(item: string, index: number) => (
					<ComboboxItem index={index} key={item} value={item}>
						<span className="min-w-0 flex-1 truncate">{optionLabel(item)}</span>
						{currentContextNames.has(item) ? <span className="text-xs text-muted-foreground">{t('common.current')}</span> : null}
					</ComboboxItem>
				)}
			</ComboboxCollection>
		</ComboboxList>
	)
}
