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
	Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
	createPVCMutationOptions,
	deletePVCMutationOptions,
	expandPVCMutationOptions,
} from '@/features/viewer/api/viewer-mutations'
import { pvcListQueryOptions, storageClassListQueryOptions, viewerContextQueryOptions } from '@/features/viewer/api/viewer-query-options'
import { ErrorCallout } from '@/features/viewer/components/error-callout'
import { PVCListSkeleton } from '@/features/viewer/components/loading-skeletons'
import { NamespaceFilter } from '@/features/viewer/components/namespace-filter'
import { PVCStatusBadge } from '@/features/viewer/components/pvc-status-badge'
import { ViewerLaunchPanel } from '@/features/viewer/components/viewer-launch-panel'
import { useViewerNamespace, useViewerSearch, useViewerView, viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'
import { formatBytes } from '@/features/viewer/utils/format-capacity'
import { deriveSessionCapability } from '@/features/viewer/utils/session-capability'
import { canLaunchViewer } from '@/features/viewer/utils/viewer-status'

interface StorageAppShellProps {
	api?: ViewerAPI
}

interface CreatePVCForm {
	accessMode: string
	capacityGi: number
	name: string
	storageClassName: string
}

const defaultCreatePVCForm: CreatePVCForm = {
	accessMode: 'ReadWriteOnce',
	capacityGi: 10,
	name: '',
	storageClassName: '__default__',
}

interface CreatePVCVariables {
	accessModes: string[]
	capacity: string
	capacityBytes: number
	name: string
	namespace: string
	storageClassName?: string
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
	const [expandPVC, setExpandPVC] = useState<PVC | null>(null)
	const [deleteState, setDeleteState] = useState<DeletePVCState | null>(null)
	const { i18n, t } = useTranslation()

	const contextQuery = useQuery(viewerContextQueryOptions(api))
	const effectiveNamespace = contextQuery.data?.namespace ?? ''
	const pvcQuery = useQuery(pvcListQueryOptions(effectiveNamespace, api))
	const storageClassesQuery = useQuery(storageClassListQueryOptions(api))
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
	const expandPVCMutation = useMutation(expandPVCMutationOptions(queryClient, api))
	const deletePVC = useMutation(deletePVCMutationOptions(queryClient, api))

	useEffect(() => {
		if (contextQuery.data?.namespace && contextQuery.data.namespace !== namespace) {
			viewerUIStore.actions.syncContextNamespace(contextQuery.data.namespace)
		}
	}, [contextQuery.data?.namespace, namespace])

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
							</TabsList>
						</Tabs>
						<div className="flex flex-col gap-2 md:ml-auto md:flex-row md:items-center">
							<NamespaceFilter />
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
	const form = useForm({
		defaultValues: defaultCreatePVCForm,
		onSubmit: ({ value }) => {
			mutation.mutate({
				namespace,
				name: value.name.trim(),
				capacity: `${value.capacityGi}Gi`,
				capacityBytes: value.capacityGi * 1024 * 1024 * 1024,
				accessModes: [value.accessMode],
				storageClassName: value.storageClassName === '__default__' ? undefined : value.storageClassName,
			}, {
				onSuccess: () => {
					toast.success(t('volumes.created'))
					form.reset(defaultCreatePVCForm)
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
					<form.Field name="storageClassName">
						{field => (
							<FormField id="pvc-storage-class" label={t('volumes.storageClass')}>
								<Select onValueChange={value => field.handleChange(value)} value={field.state.value}>
									<SelectTrigger id="pvc-storage-class">
										<SelectValue placeholder={t('volumes.defaultStorageClass')} />
									</SelectTrigger>
									<SelectContent>
										<SelectGroup>
											<SelectItem value="__default__">{t('volumes.defaultStorageClass')}</SelectItem>
											{storageClasses.map(storageClass => (
												<SelectItem key={storageClass.name} value={storageClass.name}>
													{storageClass.name}
													{storageClass.is_default ? ` · ${t('common.default')}` : ''}
												</SelectItem>
											))}
										</SelectGroup>
									</SelectContent>
								</Select>
							</FormField>
						)}
					</form.Field>
					<form.Field name="accessMode">
						{field => (
							<FormField id="pvc-access-mode" label={t('viewer.accessModes')}>
								<Select onValueChange={value => field.handleChange(value)} value={field.state.value}>
									<SelectTrigger id="pvc-access-mode">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectGroup>
											<SelectItem value="ReadWriteOnce">ReadWriteOnce</SelectItem>
											<SelectItem value="ReadOnlyMany">ReadOnlyMany</SelectItem>
											<SelectItem value="ReadWriteMany">ReadWriteMany</SelectItem>
										</SelectGroup>
									</SelectContent>
								</Select>
							</FormField>
						)}
					</form.Field>
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
