import type { UseMutationResult, UseQueryResult } from '@tanstack/react-query'
import type { FileBrowserSession } from '@/features/file-manager/types/file-manager'
import type { FileSortState } from '@/features/file-manager/utils/file-tree'

import type { ViewerFlowSnapshot } from '@/features/viewer/components/viewer-launch-panel'
import type { ViewerView } from '@/features/viewer/stores/viewer-ui-store'
import type { PVC, StorageClass, ViewerAPI, ViewerSession, ViewerToken } from '@/features/viewer/types/viewer'
import type { ManualCloseKind, ViewerFlowStatus } from '@/features/viewer/utils/session-capability'
import { FileBrowserClient } from '@sealos-storage-manager/filebrowser-client'
import { useForm } from '@tanstack/react-form'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
	Database,
	FolderOpen,
	HardDrive,
	Languages,
	MoreHorizontal,
	Plus,
	RefreshCw,
	Settings,
	Trash2,
} from 'lucide-react'
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Slider } from '@/components/ui/slider'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
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
	adminUpdateStorageClassPolicyMutationOptions,
	createPVCMutationOptions,
	deletePVCMutationOptions,
	expandPVCMutationOptions,
} from '@/features/viewer/api/viewer-mutations'
import {
	adminCapabilitiesQueryOptions,
	adminNamespaceListQueryOptions,
	adminStorageClassDescribeQueryOptions,
	adminStorageClassListQueryOptions,
	adminStorageClassYAMLQueryOptions,
	pvcListQueryOptions,
	storageClassListQueryOptions,
	viewerContextQueryOptions,
} from '@/features/viewer/api/viewer-query-options'
import { ErrorCallout } from '@/features/viewer/components/error-callout'
import { PVCListSkeleton } from '@/features/viewer/components/loading-skeletons'
import { NamespaceFilter } from '@/features/viewer/components/namespace-filter'
import { PVCStatusBadge } from '@/features/viewer/components/pvc-status-badge'
import { ViewerLaunchPanel } from '@/features/viewer/components/viewer-launch-panel'
import { useViewerNamespace, useViewerSearch, useViewerView, viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'
import { formatBytes } from '@/features/viewer/utils/format-capacity'
import { deriveSessionCapability } from '@/features/viewer/utils/session-capability'
import { canLaunchViewer } from '@/features/viewer/utils/viewer-status'

const MonacoEditor = lazy(() => import('@monaco-editor/react'))

interface StorageAppShellProps {
	api?: ViewerAPI
}

interface CreatePVCForm {
	capacityGi: number
	name: string
}

const defaultCreatePVCForm: CreatePVCForm = {
	capacityGi: 10,
	name: '',
}

const storageClassAccessModes = ['ReadWriteOnce', 'ReadOnlyMany', 'ReadWriteMany'] as const
const largeEditorDialogClassName = 'h-[88vh] max-h-[88vh] w-[min(96vw,90rem)] sm:max-w-[min(96vw,90rem)]'

interface CreatePVCVariables {
	accessModes: string[]
	capacity: string
	capacityBytes: number
	name: string
	namespace: string
	storageClassName: string
}

interface ExpandPVCVariables {
	capacity: string
	capacityBytes: number
	name: string
	namespace: string
}

interface DeletePVCVariables {
	name: string
	namespace: string
}

interface DeletePVCState {
	confirmName: string
	pvc: PVC
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
	const [storageClassPolicyEditor, setStorageClassPolicyEditor] = useState<StorageClass | null>(null)
	const [describeStorageClassName, setDescribeStorageClassName] = useState<string | null>(null)
	const [deleteStorageClassName, setDeleteStorageClassName] = useState<string | null>(null)
	const [expandPVC, setExpandPVC] = useState<PVC | null>(null)
	const [deleteState, setDeleteState] = useState<DeletePVCState | null>(null)
	const { i18n, t } = useTranslation()

	const contextQuery = useQuery(viewerContextQueryOptions(api))
	const adminCapabilitiesQuery = useQuery(adminCapabilitiesQueryOptions(api))
	const canManagePVCs = adminCapabilitiesQuery.data?.can_manage_pvcs ?? false
	const canManageStorageClasses = adminCapabilitiesQuery.data?.can_manage_storage_classes ?? false
	const adminNamespacesQuery = useQuery(adminNamespaceListQueryOptions(api, canManagePVCs))
	const effectiveNamespace = namespace || contextQuery.data?.namespace || ''
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
	const displayFileSession = fileSession ?? (
		sessionCapability.canShowFileList ? lastFileSessionRef.current : null
	)
	const showSessionNavigation = sessionCapability.canShowSessionNavigation
	const showFileNavigation = sessionCapability.canShowFileList

	const createPVC = useMutation(createPVCMutationOptions(queryClient, api))
	const createStorageClassMutation = useMutation(adminCreateStorageClassMutationOptions(queryClient, api))
	const updateStorageClassMutation = useMutation(adminUpdateStorageClassMutationOptions(queryClient, api))
	const updateStorageClassPolicyMutation = useMutation(adminUpdateStorageClassPolicyMutationOptions(queryClient, api))
	const deleteStorageClassMutation = useMutation(adminDeleteStorageClassMutationOptions(queryClient, api))
	const expandPVCMutation = useMutation(expandPVCMutationOptions(queryClient, api))
	const deletePVC = useMutation(deletePVCMutationOptions(queryClient, api))

	useEffect(() => {
		if (contextQuery.data?.namespace && !namespace) {
			viewerUIStore.actions.syncContextNamespace(contextQuery.data.namespace)
		}
	}, [contextQuery.data?.namespace, namespace])

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

	const handleReconnect = useCallback((error?: unknown) => {
		void recoverRef.current?.(error)
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
		if (!showSessionNavigation && view === 'files') {
			viewerUIStore.actions.setView('volumes')
		}
		if (!showFileNavigation && view === 'trash') {
			viewerUIStore.actions.setView(showSessionNavigation ? 'files' : 'volumes')
		}
	}, [showFileNavigation, showSessionNavigation, view])

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
						{showSessionNavigation ? <SidebarButton icon={<FolderOpen />} label={t('nav.files')} value="files" view={view} /> : null}
						{showFileNavigation ? <SidebarButton icon={<Trash2 />} label={t('nav.trash')} value="trash" view={view} /> : null}
						{canManageStorageClasses ? <SidebarButton icon={<Settings />} label={t('nav.storageClasses')} value="storageClasses" view={view} /> : null}
					</nav>
				</aside>

				<div className="flex min-w-0 flex-1 flex-col">
					<header className="flex flex-col gap-4 border-b bg-background px-4 py-4 md:flex-row md:items-center md:justify-between">
						<div className="flex min-w-0 items-center gap-3 lg:hidden">
							<div className="flex size-10 items-center justify-center rounded-lg border bg-muted">
								<Database />
							</div>
							<div className="min-w-0">
								<h1 className="text-xl font-semibold">{t('app.name')}</h1>
								<p className="text-sm text-muted-foreground">{t('app.subtitle')}</p>
							</div>
						</div>
						<Tabs
							className="lg:hidden"
							onValueChange={value => viewerUIStore.actions.setView(value as ViewerView)}
							value={view}
						>
							<TabsList>
								<TabsTrigger value="volumes">{t('nav.volumes')}</TabsTrigger>
								{showSessionNavigation ? <TabsTrigger value="files">{t('nav.files')}</TabsTrigger> : null}
								{showFileNavigation ? <TabsTrigger value="trash">{t('nav.trash')}</TabsTrigger> : null}
								{canManageStorageClasses ? <TabsTrigger value="storageClasses">{t('nav.storageClasses')}</TabsTrigger> : null}
							</TabsList>
						</Tabs>
						<div className="flex flex-col gap-2 md:ml-auto md:flex-row md:items-center">
							<NamespaceFilter
								canSelectNamespaces={canManagePVCs}
								isLoadingNamespaces={adminNamespacesQuery.isLoading}
								namespace={effectiveNamespace}
								namespaces={adminNamespaces}
								onNamespaceChange={handleNamespaceChange}
							/>
							<Button
								aria-label={t('actions.refresh')}
								disabled={!effectiveNamespace}
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
									canCreate={Boolean(effectiveNamespace)}
									createOpen={createOpen}
									onCreateOpenChange={setCreateOpen}
									onDelete={pvc => setDeleteState({ pvc, confirmName: '' })}
									onExpand={setExpandPVC}
									onOpenFiles={openFiles}
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
									onReconnect={handleReconnect}
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
									deleteMutation={deleteStorageClassMutation}
									onCreate={() => setStorageClassEditor({ mode: 'create' })}
									onDelete={setDeleteStorageClassName}
									onDescribe={setDescribeStorageClassName}
									onEdit={name => setStorageClassEditor({ mode: 'edit', name })}
									onEditPolicy={setStorageClassPolicyEditor}
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
				open={createOpen}
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
			<StorageClassPolicyDialog
				mutation={updateStorageClassPolicyMutation}
				onOpenChange={setStorageClassPolicyEditor}
				storageClass={storageClassPolicyEditor}
			/>
			<DeleteStorageClassDialog
				mutation={deleteStorageClassMutation}
				name={deleteStorageClassName}
				onOpenChange={setDeleteStorageClassName}
			/>
		</main>
	)
}

interface SidebarButtonProps {
	icon: React.ReactNode
	label: string
	value: ViewerView
	view: ViewerView
}

function SidebarButton({ icon, label, value, view }: SidebarButtonProps) {
	return (
		<Button
			className="justify-start"
			onClick={() => viewerUIStore.actions.setView(value)}
			variant={view === value ? 'secondary' : 'ghost'}
		>
			<span className="[&_svg]:size-4">{icon}</span>
			{label}
		</Button>
	)
}

interface VolumesViewProps {
	canCreate: boolean
	createOpen: boolean
	onCreateOpenChange: (open: boolean) => void
	onDelete: (pvc: PVC) => void
	onExpand: (pvc: PVC) => void
	onOpenFiles: (pvc: PVC) => void
	pvcQuery: UseQueryResult<PVC[], Error>
	pvcs: PVC[]
	storageClasses: StorageClass[]
}

function VolumesView({
	canCreate,
	onCreateOpenChange,
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
				return `${pvc.namespace} ${pvc.name} ${mountedPodNames}`.toLowerCase().includes(search)
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
				<Button disabled={!canCreate} onClick={() => onCreateOpenChange(true)}>
					<Plus data-icon="inline-start" />
					{t('volumes.create')}
				</Button>
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
											pvc={pvc}
										/>
									))}
									{filteredPVCs.length === 0
										? (
												<TableRow>
													<TableCell className="py-12 text-center text-muted-foreground" colSpan={4}>
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
	onDelete: (pvc: PVC) => void
	onExpand: (pvc: PVC) => void
	onOpenFiles: (pvc: PVC) => void
	pvc: PVC
}

function PVCRow({ onDelete, onExpand, onOpenFiles, pvc }: PVCRowProps) {
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
				<div className="flex flex-wrap gap-1">
					{pvc.access_modes.map(mode => (
						<Badge key={mode} variant="outline">{mode}</Badge>
					))}
				</div>
			</TableCell>
			<TableCell>
				<div className="flex justify-end items-center gap-2">
					<Button
						disabled={!canLaunchViewer(pvc)}
						onClick={() => onOpenFiles(pvc)}
						size="sm"
					>
						<FolderOpen data-icon="inline-start" />
						{t('files.browse')}
					</Button>
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

interface StorageClassAdminViewProps {
	deleteMutation: UseMutationResult<StorageClass, Error, string>
	onCreate: () => void
	onDelete: (name: string) => void
	onDescribe: (name: string) => void
	onEdit: (name: string) => void
	onEditPolicy: (storageClass: StorageClass) => void
	query: UseQueryResult<StorageClass[], Error>
}

function StorageClassAdminView({
	onCreate,
	onDelete,
	onDescribe,
	onEdit,
	onEditPolicy,
	query,
}: StorageClassAdminViewProps) {
	const { t } = useTranslation()
	const items = query.data ?? []

	return (
		<section className="flex flex-col gap-4">
			<header className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
				<div>
					<h2 className="text-xl font-semibold">{t('storageClasses.title')}</h2>
					<p className="text-sm text-muted-foreground">{t('storageClasses.description')}</p>
				</div>
				<div className="flex gap-2">
					<Button onClick={() => void query.refetch()} type="button" variant="outline">
						<RefreshCw data-icon="inline-start" />
						{t('actions.refresh')}
					</Button>
					<Button onClick={onCreate} type="button">
						<Plus data-icon="inline-start" />
						{t('storageClasses.create')}
					</Button>
				</div>
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
							<TableHead>{t('storageClasses.provisioner')}</TableHead>
							<TableHead>{t('storageClasses.reclaimPolicy')}</TableHead>
							<TableHead>{t('storageClasses.volumeBindingMode')}</TableHead>
							<TableHead>{t('storageClasses.allowVolumeExpansion')}</TableHead>
							<TableHead>{t('storageClasses.visibility')}</TableHead>
							<TableHead>{t('viewer.accessModes')}</TableHead>
							<TableHead className="text-right">{t('files.columns.actions')}</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{items.map(storageClass => (
							<TableRow key={storageClass.name}>
								<TableCell className="flex items-center gap-1">
									<div className="font-medium">{storageClass.name}</div>
									{storageClass.is_default ? <Badge variant="secondary">{t('common.default')}</Badge> : null}
								</TableCell>
								<TableCell>{storageClass.provisioner}</TableCell>
								<TableCell>{storageClass.reclaim_policy || '-'}</TableCell>
								<TableCell>{storageClass.volume_binding_mode || '-'}</TableCell>
								<TableCell>{storageClass.allow_volume_expansion ? t('common.yes') : t('common.no')}</TableCell>
								<TableCell>
									<Badge variant={storageClass.visible_in_create ? 'default' : 'outline'}>
										{storageClass.annotation_status}
									</Badge>
								</TableCell>
								<TableCell>
									<div className="flex flex-wrap gap-1">
										{storageClass.allowed_access_modes.length > 0
											? storageClass.allowed_access_modes.map(mode => <Badge key={mode} variant="outline">{mode}</Badge>)
											: <span className="text-sm text-muted-foreground">{t('common.empty')}</span>}
									</div>
								</TableCell>
								<TableCell>
									<div className="flex justify-end gap-2">
										<Button onClick={() => onDescribe(storageClass.name)} size="sm" type="button" variant="outline">
											{t('storageClasses.describe')}
										</Button>
										<Button onClick={() => onEdit(storageClass.name)} size="sm" type="button" variant="outline">
											{t('actions.edit')}
										</Button>
										<Button onClick={() => onEditPolicy(storageClass)} size="sm" type="button" variant="outline">
											{t('storageClasses.policy')}
										</Button>
										<Button onClick={() => onDelete(storageClass.name)} size="sm" type="button" variant="destructive">
											{t('actions.delete')}
										</Button>
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

interface StorageClassEditorDialogProps {
	api: ViewerAPI
	createMutation: UseMutationResult<StorageClass, Error, { yaml: string }>
	editor: StorageClassEditorState
	onOpenChange: (state: StorageClassEditorState) => void
	updateMutation: UseMutationResult<StorageClass, Error, { name: string, yaml: string }>
}

function StorageClassEditorDialog({
	api,
	createMutation,
	editor,
	onOpenChange,
	updateMutation,
}: StorageClassEditorDialogProps) {
	const { t } = useTranslation()
	const name = editor?.mode === 'edit' ? editor.name : null
	const yamlQuery = useQuery(adminStorageClassYAMLQueryOptions(name, api))
	const [yamlDraft, setYamlDraft] = useState({ key: '', value: '' })
	const open = editor !== null
	const isCreate = editor?.mode === 'create'
	const isPending = createMutation.isPending || updateMutation.isPending
	const editorKey = open ? (isCreate ? '__create__' : name ?? '') : ''
	const sourceYAML = isCreate ? defaultStorageClassYAML() : yamlQuery.data?.yaml ?? ''
	const yamlBody = yamlDraft.key === editorKey ? yamlDraft.value : sourceYAML

	function save() {
		if (!editor) {
			return
		}
		if (editor.mode === 'create') {
			createMutation.mutate({ yaml: yamlBody }, {
				onSuccess: () => {
					toast.success(t('storageClasses.saved'))
					onOpenChange(null)
				},
				onError: error => toast.error(translateViewerError(error, t)),
			})
			return
		}
		updateMutation.mutate({ name: editor.name, yaml: yamlBody }, {
			onSuccess: () => {
				toast.success(t('storageClasses.saved'))
				onOpenChange(null)
			},
			onError: error => toast.error(translateViewerError(error, t)),
		})
	}

	return (
		<Dialog onOpenChange={nextOpen => !isPending && onOpenChange(nextOpen ? editor : null)} open={open}>
			<DialogContent className={largeEditorDialogClassName}>
				<DialogHeader>
					<DialogTitle>{isCreate ? t('storageClasses.create') : t('storageClasses.edit')}</DialogTitle>
					<DialogDescription>{isCreate ? t('storageClasses.createDescription') : name}</DialogDescription>
				</DialogHeader>
				<div className="min-h-0 overflow-hidden rounded-md border">
					<Suspense fallback={<div className="p-4 text-sm text-muted-foreground">{t('common.loading')}</div>}>
						<MonacoEditor
							height="calc(88vh - 12rem)"
							language="yaml"
							loading={t('common.loading')}
							onChange={value => setYamlDraft({ key: editorKey, value: value ?? '' })}
							options={{
								fontSize: 13,
								minimap: { enabled: false },
								readOnly: isPending || yamlQuery.isLoading,
								scrollBeyondLastLine: false,
								wordWrap: 'on',
							}}
							value={yamlQuery.isLoading && !isCreate ? '' : yamlBody}
						/>
					</Suspense>
				</div>
				<DialogFooter>
					<Button disabled={isPending} onClick={() => onOpenChange(null)} type="button" variant="outline">
						{t('actions.cancel')}
					</Button>
					<Button disabled={isPending || (!isCreate && yamlQuery.isLoading)} onClick={save} type="button">
						{t('actions.save')}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}

function StorageClassDescribeDialog({
	api,
	name,
	onOpenChange,
}: {
	api: ViewerAPI
	name: string | null
	onOpenChange: (name: string | null) => void
}) {
	const { t } = useTranslation()
	const describeQuery = useQuery(adminStorageClassDescribeQueryOptions(name, api))

	return (
		<Dialog onOpenChange={open => onOpenChange(open ? name : null)} open={Boolean(name)}>
			<DialogContent className={largeEditorDialogClassName}>
				<DialogHeader>
					<DialogTitle>{t('storageClasses.describe')}</DialogTitle>
					<DialogDescription>{name}</DialogDescription>
				</DialogHeader>
				<div className="min-h-0 overflow-hidden rounded-md border">
					<Suspense fallback={<div className="p-4 text-sm text-muted-foreground">{t('common.loading')}</div>}>
						<MonacoEditor
							height="calc(88vh - 12rem)"
							language="yaml"
							loading={t('common.loading')}
							options={{
								fontSize: 13,
								minimap: { enabled: false },
								readOnly: true,
								scrollBeyondLastLine: false,
								wordWrap: 'on',
							}}
							value={describeQuery.data?.describe ?? ''}
						/>
					</Suspense>
				</div>
				<DialogFooter>
					<Button onClick={() => onOpenChange(null)} type="button">
						{t('actions.close')}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}

function StorageClassPolicyDialog({
	mutation,
	onOpenChange,
	storageClass,
}: {
	mutation: UseMutationResult<StorageClass, Error, { allowedAccessModes: string[], name: string, visibleInCreate: boolean }>
	onOpenChange: (storageClass: StorageClass | null) => void
	storageClass: StorageClass | null
}) {
	const { t } = useTranslation()
	const [draft, setDraft] = useState({
		key: '',
		allowedAccessModes: [] as string[],
		visibleInCreate: false,
	})
	const open = storageClass !== null
	const key = storageClass?.name ?? ''
	const allowedAccessModes = draft.key === key
		? draft.allowedAccessModes
		: storageClass?.allowed_access_modes ?? []
	const visibleInCreate = draft.key === key
		? draft.visibleInCreate
		: storageClass?.visible_in_create ?? false
	const canSave = Boolean(storageClass)
		&& !mutation.isPending
		&& (!visibleInCreate || allowedAccessModes.length > 0)

	function setVisibleInCreate(nextVisible: boolean) {
		if (!storageClass) {
			return
		}
		setDraft({
			key: storageClass.name,
			allowedAccessModes,
			visibleInCreate: nextVisible,
		})
	}

	function toggleAccessMode(mode: string, checked: boolean) {
		if (!storageClass) {
			return
		}
		const nextModes = checked
			? [...allowedAccessModes, mode]
			: allowedAccessModes.filter(item => item !== mode)
		setDraft({
			key: storageClass.name,
			allowedAccessModes: storageClassAccessModes.filter(item => nextModes.includes(item)),
			visibleInCreate,
		})
	}

	function save() {
		if (!storageClass) {
			return
		}
		mutation.mutate({
			name: storageClass.name,
			allowedAccessModes,
			visibleInCreate,
		}, {
			onSuccess: () => {
				toast.success(t('storageClasses.policySaved'))
				onOpenChange(null)
			},
			onError: error => toast.error(translateViewerError(error, t)),
		})
	}

	return (
		<Dialog onOpenChange={nextOpen => !mutation.isPending && onOpenChange(nextOpen ? storageClass : null)} open={open}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t('storageClasses.policy')}</DialogTitle>
					<DialogDescription>{storageClass?.name}</DialogDescription>
				</DialogHeader>
				<div className="flex flex-col gap-4">
					<label className="flex items-center gap-3 rounded-md border p-3 text-sm">
						<Checkbox
							checked={visibleInCreate}
							disabled={mutation.isPending}
							onCheckedChange={checked => setVisibleInCreate(checked === true)}
						/>
						<span className="font-medium">{t('storageClasses.visibleInCreate')}</span>
					</label>
					<div className="flex flex-col gap-3 rounded-md border p-3">
						<div className="text-sm font-medium">{t('viewer.accessModes')}</div>
						<div className="grid gap-3 sm:grid-cols-3">
							{storageClassAccessModes.map(mode => (
								<label key={mode} className="flex items-center gap-3 text-sm">
									<Checkbox
										checked={allowedAccessModes.includes(mode)}
										disabled={mutation.isPending}
										onCheckedChange={checked => toggleAccessMode(mode, checked === true)}
									/>
									<span>{mode}</span>
								</label>
							))}
						</div>
					</div>
					{visibleInCreate && allowedAccessModes.length === 0
						? <p className="text-sm text-destructive">{t('storageClasses.accessModeRequired')}</p>
						: null}
				</div>
				<DialogFooter>
					<Button disabled={mutation.isPending} onClick={() => onOpenChange(null)} type="button" variant="outline">
						{t('actions.cancel')}
					</Button>
					<Button disabled={!canSave} onClick={save} type="button">
						{t('actions.save')}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}

function DeleteStorageClassDialog({
	mutation,
	name,
	onOpenChange,
}: {
	mutation: UseMutationResult<StorageClass, Error, string>
	name: string | null
	onOpenChange: (name: string | null) => void
}) {
	const { t } = useTranslation()
	const [confirmState, setConfirmState] = useState({ name: null as string | null, value: '' })
	const confirmName = confirmState.name === name ? confirmState.value : ''

	return (
		<Dialog onOpenChange={open => onOpenChange(open ? name : null)} open={Boolean(name)}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t('storageClasses.deleteTitle')}</DialogTitle>
					<DialogDescription>{name ? t('storageClasses.deleteDescription', { name }) : ''}</DialogDescription>
				</DialogHeader>
				<FormField id="delete-storage-class-confirm" label={t('volumes.typeNameToConfirm')}>
					<Input
						id="delete-storage-class-confirm"
						onChange={event => setConfirmState({ name, value: event.target.value })}
						value={confirmName}
					/>
				</FormField>
				<DialogFooter>
					<Button onClick={() => onOpenChange(null)} type="button" variant="outline">
						{t('actions.cancel')}
					</Button>
					<Button
						disabled={!name || confirmName !== name || mutation.isPending}
						onClick={() => {
							if (!name) {
								return
							}
							mutation.mutate(name, {
								onSuccess: () => {
									toast.success(t('storageClasses.deleted'))
									onOpenChange(null)
								},
								onError: error => toast.error(translateViewerError(error, t)),
							})
						}}
						type="button"
						variant="destructive"
					>
						{t('actions.delete')}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}

function defaultStorageClassYAML() {
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: example
  annotations:
    storage-management.sealos.io/visible-in-create: "true"
    storage-management.sealos.io/access-modes: "ReadWriteOnce"
provisioner: example.com/provisioner
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
`
}

interface CreatePVCDialogProps {
	mutation: UseMutationResult<PVC, Error, CreatePVCVariables>
	namespace: string
	onOpenChange: (open: boolean) => void
	open: boolean
	storageClasses: StorageClass[]
}

function CreatePVCDialog({
	mutation,
	namespace,
	onOpenChange,
	open,
	storageClasses,
}: CreatePVCDialogProps) {
	const { t } = useTranslation()
	const visibleStorageClasses = useMemo(
		() => storageClasses.filter(storageClass =>
			storageClass.visible_in_create && storageClass.allowed_access_modes.length > 0,
		),
		[storageClasses],
	)
	const firstStorageClass = visibleStorageClasses[0]
	const [selection, setSelection] = useState({ accessMode: '', storageClassName: '' })
	const activeStorageClassName = visibleStorageClasses.some(storageClass => storageClass.name === selection.storageClassName)
		? selection.storageClassName
		: firstStorageClass?.name ?? ''
	const activeStorageClass = visibleStorageClasses.find(storageClass => storageClass.name === activeStorageClassName)
	const activeAccessMode = activeStorageClass?.allowed_access_modes.includes(selection.accessMode)
		? selection.accessMode
		: activeStorageClass?.allowed_access_modes[0] ?? ''
	const form = useForm({
		defaultValues: {
			...defaultCreatePVCForm,
		},
		onSubmit: ({ value }) => {
			if (!activeStorageClassName || !activeAccessMode) {
				return
			}
			mutation.mutate({
				namespace,
				name: value.name.trim(),
				capacity: `${value.capacityGi}Gi`,
				capacityBytes: value.capacityGi * 1024 * 1024 * 1024,
				accessModes: [activeAccessMode],
				storageClassName: activeStorageClassName,
			}, {
				onSuccess: () => {
					toast.success(t('volumes.created'))
					form.reset({
						...defaultCreatePVCForm,
					})
					setSelection({ accessMode: '', storageClassName: '' })
					onOpenChange(false)
				},
				onError: error => toast.error(translateViewerError(error, t)),
			})
		},
	})

	return (
		<Dialog onOpenChange={onOpenChange} open={open}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t('volumes.create')}</DialogTitle>
					<DialogDescription>{t('volumes.createDescription')}</DialogDescription>
				</DialogHeader>
				<form
					className="grid gap-4"
					onSubmit={(event) => {
						event.preventDefault()
						void form.handleSubmit()
					}}
				>
					<form.Field
						name="name"
						validators={{
							onChange: ({ value }) => value.trim().length === 0 ? t('volumes.nameRequired') : undefined,
						}}
					>
						{field => (
							<FormField
								error={field.state.meta.errorMap.onChange}
								id="pvc-name"
								label={t('volumes.name')}
							>
								<Input
									aria-invalid={field.state.meta.errorMap.onChange ? true : undefined}
									id="pvc-name"
									name={field.name}
									onBlur={field.handleBlur}
									onChange={event => field.handleChange(event.target.value)}
									value={field.state.value}
								/>
							</FormField>
						)}
					</form.Field>
					<div className="rounded-md border bg-muted px-3 py-2 text-sm">
						<span className="text-muted-foreground">
							{t('common.namespace')}
							:
							{' '}
						</span>
						<span className="font-medium">{namespace || t('common.loading')}</span>
					</div>
					<form.Field
						name="capacityGi"
						validators={{
							onChange: ({ value }) => value > 0 ? undefined : t('volumes.capacityRequired'),
						}}
					>
						{field => (
							<FormField
								error={field.state.meta.errorMap.onChange}
								id="pvc-capacity"
								label={t('viewer.capacity')}
							>
								<Input
									aria-invalid={field.state.meta.errorMap.onChange ? true : undefined}
									id="pvc-capacity"
									min={1}
									name={field.name}
									onBlur={field.handleBlur}
									onChange={event => field.handleChange(Number(event.target.value))}
									type="number"
									value={field.state.value}
								/>
							</FormField>
						)}
					</form.Field>
					<FormField id="pvc-storage-class" label={t('volumes.storageClass')}>
						<Select
							onValueChange={(value) => {
								const nextStorageClass = visibleStorageClasses.find(storageClass => storageClass.name === value)
								setSelection({
									accessMode: nextStorageClass?.allowed_access_modes[0] ?? '',
									storageClassName: value,
								})
							}}
							value={activeStorageClassName}
						>
							<SelectTrigger id="pvc-storage-class">
								<SelectValue placeholder={t('volumes.storageClass')} />
							</SelectTrigger>
							<SelectContent>
								<SelectGroup>
									{visibleStorageClasses.map(storageClass => (
										<SelectItem key={storageClass.name} value={storageClass.name}>
											{storageClass.name}
											{storageClass.is_default ? ` · ${t('common.default')}` : ''}
										</SelectItem>
									))}
								</SelectGroup>
							</SelectContent>
						</Select>
					</FormField>
					<FormField id="pvc-access-mode" label={t('viewer.accessModes')}>
						<Select
							onValueChange={value => setSelection({
								accessMode: value,
								storageClassName: activeStorageClassName,
							})}
							value={activeAccessMode}
						>
							<SelectTrigger id="pvc-access-mode">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectGroup>
									{(activeStorageClass?.allowed_access_modes ?? []).map(mode => (
										<SelectItem key={mode} value={mode}>{mode}</SelectItem>
									))}
								</SelectGroup>
							</SelectContent>
						</Select>
					</FormField>
					{visibleStorageClasses.length === 0
						? <p className="text-sm text-muted-foreground">{t('volumes.noAvailableStorageClasses')}</p>
						: null}
					<DialogFooter>
						<Button onClick={() => onOpenChange(false)} type="button" variant="outline">
							{t('actions.cancel')}
						</Button>
						<form.Subscribe selector={state => ({
							canSubmit: state.canSubmit,
							values: state.values,
						})}
						>
							{({ canSubmit, values }) => (
								<Button
									disabled={
										mutation.isPending
										|| !canSubmit
										|| !namespace
										|| values.name.trim().length === 0
										|| values.capacityGi <= 0
										|| activeStorageClassName.length === 0
										|| activeAccessMode.length === 0
									}
									type="submit"
								>
									{t('actions.create')}
								</Button>
							)}
						</form.Subscribe>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

function FormField({
	children,
	error,
	id,
	label,
}: {
	children: React.ReactNode
	error?: unknown
	id: string
	label: string
}) {
	const errorText = typeof error === 'string' ? error : ''

	return (
		<div className="grid gap-2" data-invalid={errorText ? true : undefined}>
			<Label htmlFor={id}>{label}</Label>
			{children}
			{errorText ? <p className="text-xs text-destructive">{errorText}</p> : null}
		</div>
	)
}

interface ExpandPVCDialogProps {
	mutation: UseMutationResult<PVC, Error, ExpandPVCVariables>
	onOpenChange: (pvc: PVC | null) => void
	pvc: PVC | null
}

function ExpandPVCDialog({ mutation, onOpenChange, pvc }: ExpandPVCDialogProps) {
	const { t } = useTranslation()
	const currentGi = pvc ? Math.max(1, Math.ceil(pvc.capacity_bytes / 1024 / 1024 / 1024)) : 1
	const [nextGi, setNextGi] = useState(currentGi + 10)
	const value = Math.max(nextGi, currentGi + 1)

	return (
		<Dialog
			onOpenChange={(open) => {
				if (!open) {
					onOpenChange(null)
				}
			}}
			open={pvc !== null}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t('volumes.expand')}</DialogTitle>
					<DialogDescription>{pvc ? `${pvc.namespace}/${pvc.name}` : ''}</DialogDescription>
				</DialogHeader>
				<div className="grid gap-4">
					<div className="flex justify-between gap-4 text-sm">
						<span>{t('volumes.currentCapacity')}</span>
						<span>
							{currentGi}
							{' '}
							Gi
						</span>
					</div>
					<div className="flex justify-between gap-4 text-sm">
						<span>{t('volumes.targetCapacity')}</span>
						<span>
							{value}
							{' '}
							Gi
						</span>
					</div>
					<Slider
						max={Math.max(currentGi + 500, 512)}
						min={currentGi + 1}
						onValueChange={values => setNextGi(values[0] ?? currentGi + 1)}
						step={1}
						value={[value]}
					/>
					<p className="text-sm text-muted-foreground">{t('volumes.expandHint')}</p>
				</div>
				<DialogFooter>
					<Button onClick={() => onOpenChange(null)} variant="outline">
						{t('actions.cancel')}
					</Button>
					<Button
						disabled={!pvc || mutation.isPending}
						onClick={() => {
							if (!pvc) {
								return
							}
							mutation.mutate({
								namespace: pvc.namespace,
								name: pvc.name,
								capacity: `${value}Gi`,
								capacityBytes: value * 1024 * 1024 * 1024,
							}, {
								onSuccess: () => {
									toast.success(t('volumes.expanded'))
									onOpenChange(null)
								},
								onError: error => toast.error(translateViewerError(error, t)),
							})
						}}
					>
						{t('volumes.expand')}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}

interface DeletePVCDialogProps {
	deleteState: DeletePVCState | null
	mutation: UseMutationResult<PVC, Error, DeletePVCVariables>
	onOpenChange: (state: DeletePVCState | null) => void
	onSuccess: () => void
}

function DeletePVCDialog({
	deleteState,
	mutation,
	onOpenChange,
	onSuccess,
}: DeletePVCDialogProps) {
	const { t } = useTranslation()
	const pvc = deleteState?.pvc

	return (
		<Dialog onOpenChange={open => !open && onOpenChange(null)} open={deleteState !== null}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t('volumes.deleteTitle')}</DialogTitle>
					<DialogDescription>{pvc ? t('volumes.deleteDescription', { name: pvc.name }) : ''}</DialogDescription>
				</DialogHeader>
				{pvc
					? (
							<div className="grid gap-2">
								<Label htmlFor="delete-confirm">{t('volumes.typeNameToConfirm')}</Label>
								<Input
									id="delete-confirm"
									onChange={event => onOpenChange({ pvc, confirmName: event.target.value })}
									value={deleteState.confirmName}
								/>
							</div>
						)
					: null}
				<DialogFooter>
					<Button onClick={() => onOpenChange(null)} variant="outline">
						{t('actions.cancel')}
					</Button>
					<Button
						disabled={!pvc || mutation.isPending || deleteState?.confirmName !== pvc.name}
						onClick={() => {
							if (!pvc) {
								return
							}
							mutation.mutate({
								namespace: pvc.namespace,
								name: pvc.name,
							}, {
								onSuccess: () => {
									toast.success(t('volumes.deleted'))
									onSuccess()
									onOpenChange(null)
								},
								onError: error => toast.error(translateViewerError(error, t)),
							})
						}}
						variant="destructive"
					>
						{t('actions.delete')}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}
