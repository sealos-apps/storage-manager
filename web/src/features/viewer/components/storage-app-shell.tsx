import type { PVC, ViewerAPI, ViewerToken } from '@/features/viewer/types/viewer'

import { useQuery } from '@tanstack/react-query'
import { Database, Languages, RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { viewerApi } from '@/features/viewer/api/viewer-api'
import { translateViewerError } from '@/features/viewer/api/viewer-error'
import { pvcListQueryOptions } from '@/features/viewer/api/viewer-query-options'
import { ActiveViewerFrame } from '@/features/viewer/components/active-viewer-frame'
import { ErrorCallout } from '@/features/viewer/components/error-callout'
import { PVCListSkeleton } from '@/features/viewer/components/loading-skeletons'
import { NamespaceFilter } from '@/features/viewer/components/namespace-filter'
import { PVCSummary } from '@/features/viewer/components/pvc-summary'
import { PVCTable } from '@/features/viewer/components/pvc-table'
import { ViewerLaunchPanel } from '@/features/viewer/components/viewer-launch-panel'
import { useViewerNamespace, useViewerView, viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'

interface StorageAppShellProps {
	api?: ViewerAPI
}

export function StorageAppShell({ api = viewerApi }: StorageAppShellProps) {
	const namespace = useViewerNamespace()
	const view = useViewerView()
	const [launchKey, setLaunchKey] = useState<string | null>(null)
	const [selectedPVC, setSelectedPVC] = useState<PVC | null>(null)
	const [token, setToken] = useState<ViewerToken | null>(null)
	const { i18n, t } = useTranslation()
	const pvcQuery = useQuery(pvcListQueryOptions(namespace, api))
	const pvcs = useMemo(() => pvcQuery.data ?? [], [pvcQuery.data])
	const namespaces = useMemo(() => {
		const values = new Set(['default', namespace])
		for (const pvc of pvcs) {
			values.add(pvc.namespace)
		}
		return [...values].sort()
	}, [namespace, pvcs])

	function launchPVC(pvc: PVC) {
		setSelectedPVC(pvc)
		setToken(null)
		setLaunchKey(`${pvc.uid}:${Date.now()}`)
		viewerUIStore.actions.selectPVC({
			namespace: pvc.namespace,
			pvcName: pvc.name,
			uid: pvc.uid,
		})
	}

	return (
		<main className="min-h-screen bg-background text-foreground">
			<div className="mx-auto flex min-h-screen w-full max-w-7xl flex-col px-4 py-4 md:px-6">
				<header className="flex flex-col gap-4 border-b pb-4 md:flex-row md:items-center md:justify-between">
					<div className="flex min-w-0 items-center gap-3">
						<div className="flex size-10 items-center justify-center rounded-lg border bg-muted">
							<Database className="size-5" />
						</div>
						<div className="min-w-0">
							<h1 className="text-xl font-semibold">{t('app.name')}</h1>
							<p className="text-sm text-muted-foreground">{t('app.subtitle')}</p>
						</div>
					</div>
					<div className="flex flex-col gap-2 md:flex-row md:items-center">
						<NamespaceFilter namespaces={namespaces} />
						<Button
							aria-label={t('actions.refresh')}
							onClick={() => void pvcQuery.refetch()}
							size="icon"
							variant="outline"
						>
							<RefreshCw />
						</Button>
						<Button
							aria-label="Locale"
							onClick={() => {
								const next = i18n.language === 'zh' ? 'en' : 'zh'
								void i18n.changeLanguage(next)
								viewerUIStore.actions.setLocale(next)
							}}
							size="icon"
							variant="outline"
						>
							<Languages />
						</Button>
					</div>
				</header>

				<Tabs
					className="mt-4 min-h-0 flex-1"
					onValueChange={value => viewerUIStore.actions.setView(value as typeof view)}
					value={view}
				>
					<TabsList>
						<TabsTrigger value="volumes">{t('nav.volumes')}</TabsTrigger>
						<TabsTrigger value="viewer">{t('nav.viewer')}</TabsTrigger>
						<TabsTrigger value="sessions">{t('nav.sessions')}</TabsTrigger>
					</TabsList>

					<TabsContent className="mt-4 flex flex-col gap-4" value="volumes">
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
									<>
										<PVCSummary pvcs={pvcs} />
										<PVCTable onLaunch={launchPVC} pvcs={pvcs} />
									</>
								)
							: null}
					</TabsContent>

					<TabsContent className="mt-4 grid gap-4 lg:grid-cols-[0.8fr_1.2fr]" value="viewer">
						<ViewerLaunchPanel
							api={api}
							autoStartKey={launchKey}
							pvc={selectedPVC}
							setToken={setToken}
						/>
						<ActiveViewerFrame token={token} />
					</TabsContent>

					<TabsContent className="mt-4" value="sessions">
						<ViewerLaunchPanel
							api={api}
							autoStartKey={launchKey}
							pvc={selectedPVC}
							setToken={setToken}
						/>
					</TabsContent>
				</Tabs>
			</div>
		</main>
	)
}
