import type { FileBrowserSession } from '@/features/file-manager/types/file-manager'
import type { FileSortState } from '@/features/file-manager/utils/file-tree'

import type { ViewerFlowSnapshot } from '@/features/viewer/components/viewer-launch-panel'
import type { DeletePVCState } from '@/features/viewer/components/volume-dialogs'
import type { ViewerView } from '@/features/viewer/stores/viewer-ui-store'
import type { PVC, ViewerAPI, ViewerSession, ViewerToken } from '@/features/viewer/types/viewer'
import type { ManualCloseKind, ViewerFlowStatus } from '@/features/viewer/utils/session-capability'
import { FileBrowserClient } from '@sealos-storage-manager/filebrowser-client'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
	Database,
	FolderOpen,
	HardDrive,
	Plus,
	RefreshCw,
	Settings,
	Trash2,
} from 'lucide-react'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { FileManagerView } from '@/features/file-manager/components/file-manager-view'
import { RecycleBinView } from '@/features/file-manager/components/recycle-bin-view'
import { trashRootPath } from '@/features/file-manager/utils/file-tree'
import { viewerApi } from '@/features/viewer/api/viewer-api'
import { translateViewerError } from '@/features/viewer/api/viewer-error'
import {
	adminCreateStorageClassMutationOptions,
	adminDeleteStorageClassMutationOptions,
	adminUpdateStorageClassMutationOptions,
	createPVCMutationOptions,
	deletePVCMutationOptions,
	expandPVCMutationOptions,
} from '@/features/viewer/api/viewer-mutations'
import {
	adminCapabilitiesQueryOptions,
	adminNamespaceListQueryOptions,
	adminStorageClassListQueryOptions,
	pvcListQueryOptions,
	storageClassListQueryOptions,
	viewerContextQueryOptions,
} from '@/features/viewer/api/viewer-query-options'
import { ErrorCallout } from '@/features/viewer/components/error-callout'
import { NamespaceFilter } from '@/features/viewer/components/namespace-filter'
import { StorageClassAdminView } from '@/features/viewer/components/storage-class-admin-view'
import { DeleteStorageClassDialog, StorageClassDescribeDialog, StorageClassEditorDialog } from '@/features/viewer/components/storage-class-dialogs'
import { SidebarButton } from '@/features/viewer/components/storage-navigation'
import { ViewerLaunchPanel } from '@/features/viewer/components/viewer-launch-panel'
import { CreatePVCDialog, DeletePVCDialog, ExpandPVCDialog } from '@/features/viewer/components/volume-dialogs'
import { VolumesView } from '@/features/viewer/components/volumes-view'
import { useViewerNamespace, useViewerView, viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'
import { deriveSessionCapability } from '@/features/viewer/utils/session-capability'
import { getCachedSealosAuthorization } from '@/services/sealos/sealos-authorization'
import { resolveSealosUserNamespace } from '@/services/sealos/sealos-session'

interface StorageAppShellProps {
	api?: ViewerAPI
}

type StorageClassEditorState
	= | { mode: 'create' }
		| { mode: 'edit', name: string }
		| null

interface ViewerFlowState {
	error: ViewerFlowSnapshot['error']
	isReconnecting: ViewerFlowSnapshot['isReconnecting']
	manualCloseKind: ViewerFlowSnapshot['manualCloseKind']
	status: ViewerFlowStatus
}

const idleViewerFlowState: ViewerFlowState = {
	error: null,
	isReconnecting: false,
	manualCloseKind: null,
	status: 'idle',
}

function isSealosUserNamespace(namespace: string) {
	return namespace.trim().startsWith('ns-')
}

export function StorageAppShell({ api = viewerApi }: StorageAppShellProps) {
	const namespace = useViewerNamespace()
	const view = useViewerView()
	const queryClient = useQueryClient()
	const recoverRef = useRef<ViewerFlowSnapshot['recover'] | null>(null)
	const registerManualCloseRef = useRef<ViewerFlowSnapshot['registerManualClose'] | null>(null)
	const lastFileSessionRef = useRef<FileBrowserSession | null>(null)
	const [launchKey, setLaunchKey] = useState<string | null>(null)
	const [selectedPVC, setSelectedPVC] = useState<PVC | null>(null)
	const [token, setToken] = useState<ViewerToken | null>(null)
	const [viewerSession, setViewerSession] = useState<ViewerSession | null>(null)
	const [viewerFlow, setViewerFlow] = useState<ViewerFlowState>(idleViewerFlowState)
	const [currentPath, setCurrentPath] = useState('/')
	const [sort, setSort] = useState<FileSortState>({ field: 'name', direction: 'asc' })
	const [createOpen, setCreateOpen] = useState(false)
	const [storageClassEditor, setStorageClassEditor] = useState<StorageClassEditorState>(null)
	const [describeStorageClassName, setDescribeStorageClassName] = useState<string | null>(null)
	const [deleteStorageClassName, setDeleteStorageClassName] = useState<string | null>(null)
	const [expandPVC, setExpandPVC] = useState<PVC | null>(null)
	const [deleteState, setDeleteState] = useState<DeletePVCState | null>(null)
	const { t } = useTranslation()

	const contextQuery = useQuery(viewerContextQueryOptions(api))
	const adminCapabilitiesQuery = useQuery(adminCapabilitiesQueryOptions(api))
	const authorization = getCachedSealosAuthorization()
	const sealosUserNamespace = resolveSealosUserNamespace(authorization?.session ?? null)
	const contextNamespace = contextQuery.data?.namespace ?? ''
	const ownNamespace = adminCapabilitiesQuery.data?.user_namespace || sealosUserNamespace || contextNamespace
	const fallbackNamespace = contextQuery.data
		? (isSealosUserNamespace(contextNamespace) ? contextNamespace : ownNamespace)
		: ''
	const canSelectAdminNamespace = adminCapabilitiesQuery.data?.can_manage_pvcs ?? false
	const canManageStorageClasses = adminCapabilitiesQuery.data?.can_manage_storage_classes ?? false
	const pvcCreationEnabled = adminCapabilitiesQuery.data?.pvc_creation_enabled ?? true
	const effectiveNamespace = canSelectAdminNamespace ? (namespace || ownNamespace) : fallbackNamespace
	const fileManagementEnabled = adminCapabilitiesQuery.data?.file_management_enabled ?? true
	const adminNamespacesQuery = useQuery(adminNamespaceListQueryOptions(api, canSelectAdminNamespace))
	const adminNamespaces = adminNamespacesQuery.data ?? []
	const pvcQuery = useQuery(pvcListQueryOptions(effectiveNamespace, api))
	const storageClassesQuery = useQuery(storageClassListQueryOptions(api))
	const adminStorageClassesQuery = useQuery(adminStorageClassListQueryOptions(api, canManageStorageClasses))
	const pvcs = useMemo(() => pvcQuery.data ?? [], [pvcQuery.data])
	const fileSession = useMemo<FileBrowserSession | null>(() => {
		if (!token || !selectedPVC || viewerSession?.status !== 'ready' || !viewerSession.token_ready) {
			return null
		}
		return {
			client: new FileBrowserClient({
				baseUrl: token.viewer_url,
				token: token.token,
			}),
			pvcKey: selectedPVC.uid,
		}
	}, [selectedPVC, token, viewerSession])
	const sessionCapability = useMemo(
		() => deriveSessionCapability({
			error: viewerFlow.error,
			isReconnecting: viewerFlow.isReconnecting,
			manualCloseKind: viewerFlow.manualCloseKind,
			selectedPVC,
			session: viewerSession,
			status: viewerFlow.status,
			token,
		}),
		[selectedPVC, token, viewerFlow, viewerSession],
	)
	const displayFileSession = (() => {
		if (!fileManagementEnabled) {
			return null
		}
		return fileSession ?? (sessionCapability.canShowFileList ? lastFileSessionRef.current : null)
	})()
	const showSessionNavigation = sessionCapability.canShowSessionNavigation
	const showFileNavigation = fileManagementEnabled && sessionCapability.canShowFileList
	const showFileEntry = fileManagementEnabled && showSessionNavigation

	const createPVC = useMutation(createPVCMutationOptions(queryClient, api))
	const createStorageClassMutation = useMutation(adminCreateStorageClassMutationOptions(queryClient, api))
	const updateStorageClassMutation = useMutation(adminUpdateStorageClassMutationOptions(queryClient, api))
	const deleteStorageClassMutation = useMutation(adminDeleteStorageClassMutationOptions(queryClient, api))
	const expandPVCMutation = useMutation(expandPVCMutationOptions(queryClient, api))
	const deletePVC = useMutation(deletePVCMutationOptions(queryClient, api))

	useEffect(() => {
		const syncNamespace = canSelectAdminNamespace ? ownNamespace : fallbackNamespace
		if (!syncNamespace) {
			return
		}
		if (!canSelectAdminNamespace || !namespace) {
			viewerUIStore.actions.syncContextNamespace(syncNamespace)
		}
	}, [canSelectAdminNamespace, fallbackNamespace, namespace, ownNamespace])

	function resetFileSessionState() {
		setSelectedPVC(null)
		setToken(null)
		setViewerSession(null)
		lastFileSessionRef.current = null
		setViewerFlow(idleViewerFlowState)
		setCurrentPath('/')
		setLaunchKey(null)
	}

	function handleNamespaceChange(nextNamespace: string) {
		if (nextNamespace === namespace) {
			return
		}
		resetFileSessionState()
		viewerUIStore.actions.setNamespace(nextNamespace)
	}

	function openFiles(pvc: PVC) {
		setSelectedPVC(pvc)
		setToken(null)
		setViewerSession(null)
		lastFileSessionRef.current = null
		setViewerFlow(idleViewerFlowState)
		setCurrentPath('/')
		setLaunchKey(`${pvc.uid}:${Date.now()}`)
		viewerUIStore.actions.selectPVC({
			namespace: pvc.namespace,
			pvcName: pvc.name,
			uid: pvc.uid,
		})
	}

	function refreshActiveSession() {
		if (!selectedPVC) {
			return
		}
		setLaunchKey(`${selectedPVC.uid}:${Date.now()}`)
	}

	function refreshAllStorageData() {
		const refetches: Array<Promise<unknown>> = [
			contextQuery.refetch(),
			adminCapabilitiesQuery.refetch(),
			pvcQuery.refetch(),
			storageClassesQuery.refetch(),
		]
		if (canSelectAdminNamespace) {
			refetches.push(adminNamespacesQuery.refetch())
		}
		if (canManageStorageClasses) {
			refetches.push(adminStorageClassesQuery.refetch())
		}
		void Promise.allSettled(refetches)
	}

	const handleFlowChange = useCallback((flow: ViewerFlowSnapshot) => {
		recoverRef.current = flow.recover
		registerManualCloseRef.current = flow.registerManualClose
		setViewerSession(flow.session)
		if (flow.manualCloseKind) {
			lastFileSessionRef.current = null
		}
		setViewerFlow(current => (
			current.error === flow.error
			&& current.isReconnecting === flow.isReconnecting
			&& current.manualCloseKind === flow.manualCloseKind
			&& current.status === flow.status
				? current
				: {
						error: flow.error,
						isReconnecting: flow.isReconnecting,
						manualCloseKind: flow.manualCloseKind,
						status: flow.status,
					}
		))
	}, [])

	const handleManualClose = useCallback((kind: ManualCloseKind) => {
		registerManualCloseRef.current?.(kind)
	}, [])

	useEffect(() => {
		if (fileSession) {
			lastFileSessionRef.current = fileSession
		}
	}, [fileSession])

	useEffect(() => {
		if ((!showSessionNavigation || !fileManagementEnabled) && view === 'files') {
			viewerUIStore.actions.setView('volumes')
		}
		if ((!showFileNavigation || !fileManagementEnabled) && view === 'trash') {
			viewerUIStore.actions.setView(showFileEntry ? 'files' : 'volumes')
		}
		if (!canManageStorageClasses && view === 'storageClasses') {
			viewerUIStore.actions.setView('volumes')
		}
	}, [canManageStorageClasses, fileManagementEnabled, showFileEntry, showFileNavigation, showSessionNavigation, view])

	const pageActions = (
		<div className="flex w-full flex-col gap-2 sm:flex-row sm:items-center lg:w-auto">
			<NamespaceFilter
				canSelectNamespaces={canSelectAdminNamespace}
				isLoadingNamespaces={adminNamespacesQuery.isLoading}
				namespace={effectiveNamespace}
				namespaces={adminNamespaces}
				onNamespaceChange={handleNamespaceChange}
			/>
			<Button
				aria-label={t('actions.refresh')}
				disabled={!effectiveNamespace}
				onClick={refreshAllStorageData}
				type="button"
				variant="outline"
			>
				<RefreshCw data-icon="inline-start" />
				{t('actions.refresh')}
			</Button>
			{view === 'storageClasses'
				? (
						<Button onClick={() => setStorageClassEditor({ mode: 'create' })} type="button">
							<Plus data-icon="inline-start" />
							{t('storageClasses.create')}
						</Button>
					)
				: null}
			{view === 'volumes' && pvcCreationEnabled
				? (
						<Button disabled={!effectiveNamespace} onClick={() => setCreateOpen(true)} type="button">
							<Plus data-icon="inline-start" />
							{t('volumes.create')}
						</Button>
					)
				: null}
		</div>
	)

	return (
		<main className="min-h-screen bg-muted/30 text-foreground">
			<div className="flex min-h-screen">
				<aside className="hidden w-64 shrink-0 border-r bg-sidebar px-4 py-5 text-sidebar-foreground lg:flex lg:flex-col">
					<div className="flex items-center gap-3 px-2">
						<div className="flex size-10 items-center justify-center rounded-lg border bg-background text-foreground shrink-0">
							<Database />
						</div>
						<div className="min-w-0">
							<h1 className="truncate text-base font-semibold">{t('app.name')}</h1>
							<p className="text-xs text-muted-foreground">{t('app.subtitle')}</p>
						</div>
					</div>
					<nav className="mt-8 flex flex-col gap-2">
						<SidebarButton icon={<HardDrive />} label={t('nav.volumes')} value="volumes" view={view} />
						{showFileEntry ? <SidebarButton icon={<FolderOpen />} label={t('nav.files')} value="files" view={view} /> : null}
						{showFileNavigation ? <SidebarButton icon={<Trash2 />} label={t('nav.trash')} value="trash" view={view} /> : null}
						{canManageStorageClasses ? <SidebarButton icon={<Settings />} label={t('nav.storageClasses')} value="storageClasses" view={view} /> : null}
					</nav>
				</aside>

				<div className="flex min-w-0 flex-1 flex-col">
					<header className="border-b bg-background px-4 py-4 lg:hidden">
						<Tabs
							onValueChange={value => viewerUIStore.actions.setView(value as ViewerView)}
							value={view}
						>
							<TabsList>
								<TabsTrigger value="volumes">{t('nav.volumes')}</TabsTrigger>
								{showFileEntry ? <TabsTrigger value="files">{t('nav.files')}</TabsTrigger> : null}
								{showFileNavigation ? <TabsTrigger value="trash">{t('nav.trash')}</TabsTrigger> : null}
								{canManageStorageClasses ? <TabsTrigger value="storageClasses">{t('nav.storageClasses')}</TabsTrigger> : null}
							</TabsList>
						</Tabs>
					</header>

					<div className="min-h-0 flex-1 px-4 py-4">
						{contextQuery.error
							? (
									<div className="mb-4">
										<ErrorCallout title={t('common.error')}>
											{translateViewerError(contextQuery.error, t)}
										</ErrorCallout>
									</div>
								)
							: null}
						<Tabs
							className="h-full"
							onValueChange={value => viewerUIStore.actions.setView(value as ViewerView)}
							value={view}
						>
							<TabsContent className="m-0 flex h-full flex-col gap-4" value="volumes">
								<VolumesView
									actions={pageActions}
									onDelete={pvc => setDeleteState({ pvc, confirmName: '' })}
									onExpand={setExpandPVC}
									onOpenFiles={openFiles}
									fileManagementEnabled={fileManagementEnabled}
									pvcQuery={pvcQuery}
									pvcs={pvcs}
									storageClasses={storageClassesQuery.data ?? []}
								/>
							</TabsContent>
							<TabsContent className="m-0 flex h-full min-h-0 flex-col gap-4" value="files">
								<ViewerLaunchPanel
									api={api}
									autoStartKey={launchKey}
									onFlowChange={handleFlowChange}
									pvc={selectedPVC}
									setToken={setToken}
								/>
								<FileManagerView
									api={api}
									currentPath={currentPath}
									onBackToVolumes={() => viewerUIStore.actions.setView('volumes')}
									onManualClose={handleManualClose}
									onPathChange={(path) => {
										if (path !== trashRootPath) {
											setCurrentPath(path)
										}
									}}
									onRefreshSession={refreshActiveSession}
									podSessionID={viewerSession?.pod_session_id ?? null}
									pvcName={selectedPVC?.name}
									session={displayFileSession}
									sessionCapability={sessionCapability}
									setSort={setSort}
									sort={sort}
									viewerSession={viewerSession}
									viewerSessionID={viewerSession?.id ?? null}
								/>
							</TabsContent>
							<TabsContent className="m-0 flex h-full min-h-0 flex-col" value="trash">
								<RecycleBinView session={fileSession} />
							</TabsContent>
							<TabsContent className="m-0 flex h-full min-h-0 flex-col" value="storageClasses">
								<StorageClassAdminView
									actions={pageActions}
									deleteMutation={deleteStorageClassMutation}
									onDelete={setDeleteStorageClassName}
									onDescribe={setDescribeStorageClassName}
									onEdit={name => setStorageClassEditor({ mode: 'edit', name })}
									query={adminStorageClassesQuery}
								/>
							</TabsContent>
						</Tabs>
					</div>
				</div>
			</div>

			<CreatePVCDialog
				namespace={effectiveNamespace}
				mutation={createPVC}
				onOpenChange={setCreateOpen}
				open={createOpen && pvcCreationEnabled}
				storageClasses={storageClassesQuery.data ?? []}
			/>
			<ExpandPVCDialog
				mutation={expandPVCMutation}
				onOpenChange={setExpandPVC}
				pvc={expandPVC}
			/>
			<DeletePVCDialog
				deleteState={deleteState}
				mutation={deletePVC}
				onOpenChange={setDeleteState}
				onSuccess={() => {
					if (deleteState?.pvc.uid === selectedPVC?.uid) {
						setSelectedPVC(null)
						setToken(null)
						setViewerSession(null)
						lastFileSessionRef.current = null
						setViewerFlow(idleViewerFlowState)
					}
				}}
			/>
			<StorageClassEditorDialog
				createMutation={createStorageClassMutation}
				editor={storageClassEditor}
				onOpenChange={setStorageClassEditor}
				updateMutation={updateStorageClassMutation}
				api={api}
			/>
			<StorageClassDescribeDialog
				api={api}
				name={describeStorageClassName}
				onOpenChange={setDescribeStorageClassName}
			/>
			<DeleteStorageClassDialog
				mutation={deleteStorageClassMutation}
				name={deleteStorageClassName}
				onOpenChange={setDeleteStorageClassName}
			/>
		</main>
	)
}
