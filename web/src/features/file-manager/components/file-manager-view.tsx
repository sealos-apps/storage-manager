import type { FileBrowserResource } from '@sealos-storage-manager/filebrowser-client'
import type { UseQueryResult } from '@tanstack/react-query'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import type { FileBrowserSession, FileEntry, FileListResult, FileTableRow, FileUsage } from '@/features/file-manager/types/file-manager'
import type { FileSortState } from '@/features/file-manager/utils/file-tree'
import type { ViewerAPI, ViewerSession } from '@/features/viewer/types/viewer'
import type { ManualCloseKind, SessionCapability } from '@/features/viewer/utils/session-capability'

import { parentPath } from '@sealos-storage-manager/filebrowser-client'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { flexRender, getCoreRowModel, getSortedRowModel, useReactTable } from '@tanstack/react-table'
import {
	ArrowLeft,
	ChevronDown,
	ChevronRight,
	Download,
	Edit3,
	File,
	Folder,
	FolderPlus,
	Info,
	Loader2,
	RefreshCw,
	Trash2,
	Upload,
} from 'lucide-react'
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
	Popover,
	PopoverContent,
	PopoverDescription,
	PopoverHeader,
	PopoverTitle,
	PopoverTrigger,
} from '@/components/ui/popover'
import { Progress } from '@/components/ui/progress'
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
import { invalidateFileTreeQueries } from '@/features/file-manager/api/file-manager-cache'
import {
	createFolderMutationOptions,
	createUploadTaskID,
	moveToRecycleBinMutationOptions,
	saveFileTextMutationOptions,
	uploadFileMutationOptions,
} from '@/features/file-manager/api/file-manager-mutations'
import { fileListQueryOptions, fileTextQueryOptions, fileUsageQueryOptions } from '@/features/file-manager/api/file-manager-query-options'
import { uploadActions, useUploadTask, useUploadTasks } from '@/features/file-manager/stores/upload-store'
import {
	buildFileTableRows,
	flattenResources,
	isEditableFile,
	nextSortState,
	sortEntries,
} from '@/features/file-manager/utils/file-tree'
import { viewerApi } from '@/features/viewer/api/viewer-api'
import { translateViewerError } from '@/features/viewer/api/viewer-error'
import { SessionActions } from '@/features/viewer/components/session-actions'
import { formatBytes } from '@/features/viewer/utils/format-capacity'
import { cn } from '@/utils/cn'

interface FileManagerViewProps {
	api?: ViewerAPI
	currentPath: string
	onBackToVolumes: () => void
	onManualClose?: (kind: ManualCloseKind) => void
	onPathChange: (path: string) => void
	onReconnect: (error?: unknown) => void
	onRefreshSession: () => void
	podSessionID?: string | null
	pvcName?: string
	session: FileBrowserSession | null
	sessionCapability: SessionCapability
	sort: FileSortState
	setSort: (sort: FileSortState) => void
	viewerSession?: ViewerSession | null
	viewerSessionID?: string | null
}

interface BranchState {
	entries?: FileEntry[]
	error?: Error
	isLoading?: boolean
}

interface BranchTreeState {
	expandedDepths: Record<string, number | undefined>
	scope: string
}

interface BranchQuerySnapshot {
	data?: FileBrowserResource
	error: Error | null
	isFetching: boolean
	isLoading: boolean
}

const emptyEntries: FileEntry[] = []
const emptyBranches: Record<string, BranchState | undefined> = {}
const emptyExpandedDepths: Record<string, number | undefined> = {}
const maxEditableFileBytes = 32 * 1024 * 1024
const MonacoEditor = lazy(() => import('@monaco-editor/react'))

function createBranchTreeState(scope: string): BranchTreeState {
	return {
		expandedDepths: {},
		scope,
	}
}

export function FileManagerView({
	api = viewerApi,
	currentPath,
	onBackToVolumes,
	onManualClose,
	onPathChange,
	onReconnect,
	onRefreshSession,
	podSessionID,
	pvcName,
	session,
	sessionCapability,
	sort,
	setSort,
	viewerSession,
	viewerSessionID,
}: FileManagerViewProps) {
	const { t } = useTranslation()
	const queryClient = useQueryClient()
	const canShowFileList = sessionCapability.canShowFileList && session !== null
	const canUseFiles = sessionCapability.canUseFiles && session !== null
	const fileQuery = useQuery(fileListQueryOptions(session, currentPath, sort, canUseFiles))
	const usageQuery = useQuery(fileUsageQueryOptions(session, canUseFiles))
	const entries = fileQuery.data?.entries ?? emptyEntries
	const treeScope = `${session?.pvcKey ?? 'inactive'}:${currentPath}`
	const [treeState, setTreeState] = useState<BranchTreeState>(() => createBranchTreeState(treeScope))
	const expandedDepths = treeState.scope === treeScope ? treeState.expandedDepths : emptyExpandedDepths
	const expandedPathList = useMemo(() => Object.keys(expandedDepths), [expandedDepths])
	const expandedPaths = useMemo(() => new Set(expandedPathList), [expandedPathList])
	const branchQueryOptions = useMemo(
		() => expandedPathList.map(path => fileListQueryOptions(session, path, sort, canUseFiles)),
		[canUseFiles, expandedPathList, session, sort],
	)
	const branchQueries = useQueries({
		queries: branchQueryOptions,
		combine: useCallback((results: UseQueryResult<FileListResult, Error>[]) =>
			results.map((result): BranchQuerySnapshot => ({
				data: result.data?.current,
				error: result.error instanceof Error ? result.error : null,
				isFetching: result.isFetching,
				isLoading: result.isLoading,
			})), []),
	})
	const branches = useMemo(() => {
		if (expandedPathList.length === 0) {
			return emptyBranches
		}
		const nextBranches: Record<string, BranchState | undefined> = {}
		expandedPathList.forEach((path, index) => {
			const query = branchQueries[index]
			const depth = expandedDepths[path] ?? 0
			if (!query || query.isLoading || query.isFetching) {
				nextBranches[path] = { isLoading: true }
				return
			}
			if (query.error) {
				nextBranches[path] = { error: query.error }
				return
			}
			nextBranches[path] = {
				entries: query.data
					? sortEntries(flattenResources(query.data, depth + 1), sort)
					: [],
			}
		})
		return nextBranches
	}, [branchQueries, expandedDepths, expandedPathList, sort])

	const operationsDisabled = !canUseFiles || fileQuery.isFetching || hasPendingBranches(branches)
	const showOverlay = canShowFileList && (fileQuery.isFetching || sessionCapability.kind === 'viewer-reconnecting')
	const visiblePath = fileQuery.data?.path ?? currentPath

	useEffect(() => {
		if (fileQuery.error) {
			onReconnect(fileQuery.error)
		}
	}, [fileQuery.error, onReconnect])

	const toggleFolder = useCallback((entry: FileEntry) => {
		if (!session || operationsDisabled) {
			return
		}

		setTreeState((current) => {
			const scoped = current.scope === treeScope ? current : createBranchTreeState(treeScope)
			const next = { ...scoped.expandedDepths }
			if (next[entry.path] !== undefined) {
				delete next[entry.path]
			}
			else {
				next[entry.path] = entry.depth
			}
			return {
				...scoped,
				expandedDepths: next,
			}
		})
	}, [operationsDisabled, session, treeScope])

	const [editingEntry, setEditingEntry] = useState<FileEntry | null>(null)
	const openEntry = useCallback((entry: FileEntry) => {
		if (entry.isDir) {
			onPathChange(entry.path)
			return
		}
		if (isEditableFile(entry.path)) {
			if (entry.size > maxEditableFileBytes) {
				toast.error(t('files.editorTooLarge', { size: formatBytes(maxEditableFileBytes) }))
				return
			}
			setEditingEntry(entry)
		}
	}, [onPathChange, t])

	const rows = useMemo(
		() => buildFileTableRows(entries, expandedPaths, branches),
		[branches, entries, expandedPaths],
	)

	const columns = useMemo<ColumnDef<FileTableRow>[]>(() => [
		{
			accessorFn: row => row.kind === 'resource' ? row.entry.name : row.path,
			cell: info => (
				<FileNameCell
					disabled={operationsDisabled}
					isExpanded={info.row.original.kind === 'resource' && expandedPaths.has(info.row.original.entry.path)}
					isLoading={info.row.original.kind === 'resource' && Boolean(branches[info.row.original.entry.path]?.isLoading)}
					onOpen={entry => openEntry(entry)}
					onRetryBranch={(path) => {
						if (session) {
							invalidateFileTreeQueries(queryClient, session, [path])
						}
					}}
					onToggleFolder={entry => void toggleFolder(entry)}
					row={info.row.original}
				/>
			),
			header: () => (
				<SortableHead
					active={sort.field === 'name'}
					disabled={operationsDisabled}
					direction={sort.direction}
					label={t('files.columns.name')}
					onClick={() => setSort(nextSortState(sort, 'name'))}
				/>
			),
			id: 'name',
		},
		{
			accessorFn: row => row.kind === 'resource' ? row.entry.size : 0,
			cell: info => info.row.original.kind === 'resource'
				? <span className="block truncate">{info.row.original.entry.isDir ? '-' : formatBytes(info.row.original.entry.size)}</span>
				: '',
			header: () => (
				<SortableHead
					active={sort.field === 'size'}
					disabled={operationsDisabled}
					direction={sort.direction}
					label={t('files.columns.size')}
					onClick={() => setSort(nextSortState(sort, 'size'))}
				/>
			),
			id: 'size',
		},
		{
			accessorFn: row => row.kind === 'resource' ? row.entry.modified : '',
			cell: info => info.row.original.kind === 'resource'
				? <ModifiedTimeCell value={info.row.original.entry.modified} />
				: '',
			header: () => (
				<SortableHead
					active={sort.field === 'modified'}
					disabled={operationsDisabled}
					direction={sort.direction}
					label={t('files.columns.modified')}
					onClick={() => setSort(nextSortState(sort, 'modified'))}
				/>
			),
			id: 'modified',
		},
		{
			cell: info => info.row.original.kind === 'resource' && session
				? (
						<FileActions
							disabled={operationsDisabled}
							entry={info.row.original.entry}
							onOpenFolder={onPathChange}
							session={session}
						/>
					)
				: null,
			header: () => <span>{t('files.columns.actions')}</span>,
			id: 'actions',
		},
	], [branches, expandedPaths, onPathChange, openEntry, operationsDisabled, queryClient, session, setSort, sort, t, toggleFolder])

	const table = useReactTable({
		columns,
		data: rows,
		getCoreRowModel: getCoreRowModel(),
		getRowId: row => row.id,
		getSortedRowModel: getSortedRowModel(),
		manualSorting: true,
		state: {
			sorting: [{ desc: sort.direction === 'desc', id: sort.field }] satisfies SortingState,
		},
	})

	return (
		<section className="flex min-h-0 flex-1 flex-col gap-4">
			<header className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
				<div className="min-w-0">
					<h2 className="text-xl font-semibold">{t('files.title')}</h2>
					<p className="text-sm text-muted-foreground">
						{pvcName ? t('files.subtitle', { pvc: pvcName }) : t('files.noSelection')}
					</p>
				</div>
				<div className="flex flex-wrap items-center gap-3">
					<SessionStatusPopover
						api={api}
						onManualClose={onManualClose}
						onRefreshSession={onRefreshSession}
						podSessionID={podSessionID ?? null}
						session={viewerSession ?? null}
						sessionCapability={sessionCapability}
						viewerSessionID={viewerSessionID ?? null}
					/>
					{canShowFileList ? <StorageUsageSummary query={usageQuery} /> : null}
					<Button onClick={onBackToVolumes} size="sm" variant="outline">
						<ArrowLeft data-icon="inline-start" />
						{t('files.backToVolumes')}
					</Button>
					{canShowFileList
						? (
								<>
									<CreateFolderDialog
										currentPath={currentPath}
										disabled={operationsDisabled}
										session={session}
									/>
									<UploadDialog
										currentPath={currentPath}
										disabled={operationsDisabled}
										podSessionID={podSessionID}
										session={session}
										viewerSessionID={viewerSessionID}
									/>
									<Button
										aria-label={t('actions.refresh')}
										disabled={!canUseFiles || operationsDisabled}
										onClick={() => {
											onRefreshSession()
											void usageQuery.refetch()
											void fileQuery.refetch()
										}}
										size="icon"
										variant="outline"
									>
										<RefreshCw />
									</Button>
								</>
							)
						: null}
				</div>
			</header>
			<Separator />

			{!canShowFileList
				? (
						<div className="rounded-lg border bg-card p-6 text-sm text-muted-foreground">
							{t(sessionCapability.messageKey)}
						</div>
					)
				: (
						<>
							<div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
								<Button
									disabled={currentPath === '/' || operationsDisabled}
									onClick={() => onPathChange(parentPath(currentPath))}
									size="sm"
									variant="ghost"
								>
									<ArrowLeft data-icon="inline-start" />
									{t('files.up')}
								</Button>
								<span className="rounded-md border bg-muted px-2 py-1 font-mono text-xs text-foreground">
									{visiblePath}
								</span>
								{currentPath !== visiblePath
									? (
											<span className="rounded-md border bg-muted px-2 py-1 text-xs">
												{t('files.pendingPath', { path: currentPath })}
											</span>
										)
									: null}
								{!canUseFiles ? <span>{t(sessionCapability.messageKey)}</span> : null}
							</div>

							<div className="relative min-h-0 rounded-lg border bg-card">
								<Table className="table-fixed">
									<colgroup>
										<col className="w-[52%]" />
										<col className="w-28" />
										<col className="w-44" />
										<col className="w-36" />
									</colgroup>
									<TableHeader>
										{table.getHeaderGroups().map(headerGroup => (
											<TableRow key={headerGroup.id}>
												{headerGroup.headers.map(header => (
													<TableHead className={tableColumnClassName(header.id)} key={header.id}>
														{header.isPlaceholder
															? null
															: flexRender(header.column.columnDef.header, header.getContext())}
													</TableHead>
												))}
											</TableRow>
										))}
									</TableHeader>
									<TableBody>
										{fileQuery.isLoading && rows.length === 0
											? (
													<TableRow>
														<TableCell className="py-12 text-center text-muted-foreground" colSpan={4}>
															{t('files.pending')}
														</TableCell>
													</TableRow>
												)
											: null}
										{fileQuery.error && rows.length === 0
											? (
													<TableRow>
														<TableCell className="py-10" colSpan={4}>
															<FileListErrorState
																api={api}
																error={fileQuery.error}
																onManualClose={onManualClose}
																onRetry={() => void fileQuery.refetch()}
																podSessionID={podSessionID ?? null}
																sessionCapability={sessionCapability}
																viewerSessionID={viewerSessionID ?? null}
															/>
														</TableCell>
													</TableRow>
												)
											: null}
										{table.getRowModel().rows.map(row => (
											<TableRow key={row.id}>
												{row.getVisibleCells().map(cell => (
													<TableCell className={tableColumnClassName(cell.column.id)} key={cell.id}>
														{flexRender(cell.column.columnDef.cell, cell.getContext())}
													</TableCell>
												))}
											</TableRow>
										))}
										{!fileQuery.isLoading && !fileQuery.error && rows.length === 0
											? (
													<TableRow>
														<TableCell className="py-12 text-center text-muted-foreground" colSpan={4}>
															{t('files.empty')}
														</TableCell>
													</TableRow>
												)
											: null}
									</TableBody>
								</Table>
								{showOverlay
									? (
											<div className="absolute inset-0 flex items-center justify-center rounded-lg bg-background/70 backdrop-blur-[1px]" role="status">
												<div className="rounded-md border bg-card px-4 py-3 text-sm text-muted-foreground shadow-sm">
													{sessionCapability.kind === 'viewer-reconnecting'
														? t('files.reconnecting')
														: t('files.pending')}
												</div>
											</div>
										)
									: null}
							</div>
						</>
					)}

			<UploadTaskList />
			{session && editingEntry
				? (
						<FileEditorDialog
							entry={editingEntry}
							onOpenChange={(open) => {
								if (!open) {
									setEditingEntry(null)
								}
							}}
							open={true}
							session={session}
						/>
					)
				: null}
		</section>
	)
}

function UploadTaskList() {
	const { t } = useTranslation()
	const tasks = useUploadTasks()

	if (tasks.length === 0) {
		return null
	}

	return (
		<div className="grid gap-2 rounded-lg border bg-card p-3">
			<div className="flex items-center justify-between gap-3">
				<div className="text-sm font-medium">{t('files.uploadTasks')}</div>
				<Button onClick={() => uploadActions.clearCompleted()} size="sm" variant="ghost">
					{t('files.clearCompleted')}
				</Button>
			</div>
			{tasks.map(task => (
				<UploadTaskRow key={task.id} taskID={task.id} />
			))}
		</div>
	)
}

interface SessionStatusPopoverProps {
	api: ViewerAPI
	onRefreshSession: () => void
	onManualClose?: (kind: ManualCloseKind) => void
	podSessionID: string | null
	session: ViewerSession | null
	sessionCapability: SessionCapability
	viewerSessionID: string | null
}

function SessionStatusPopover({
	api,
	onManualClose,
	onRefreshSession,
	podSessionID,
	session,
	sessionCapability,
	viewerSessionID,
}: SessionStatusPopoverProps) {
	const { t } = useTranslation()
	const statusClassName = sessionStatusDotClassName(sessionCapability.kind)
	const canRetry = sessionCapability.kind !== 'viewer-ready'

	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button aria-label={t('files.sessionStatus')} title={t('files.sessionStatus')} size="icon" variant="outline">
					<span className={cn('block size-2.5 rounded-full', statusClassName)} />
				</Button>
			</PopoverTrigger>
			<PopoverContent align="end" className="w-[min(calc(100vw-2rem),30rem)]">
				<PopoverHeader>
					<PopoverTitle className="flex items-center gap-2">
						<span className={cn('block size-2.5 rounded-full', statusClassName)} />
						{t('files.sessionStatus')}
					</PopoverTitle>
					<PopoverDescription>
						{t(sessionCapability.messageKey)}
					</PopoverDescription>
				</PopoverHeader>
				<div className="mt-4 flex flex-col gap-3 text-sm">
					<SessionDetailRow label={t('status.label')} value={session ? t(`status.${session.status}`, { defaultValue: session.status }) : t('status.idle')} />
					<SessionDetailRow label={t('viewer.podSession')} value={session?.pod_session_id ?? podSessionID ?? '-'} />
					<SessionDetailRow label={t('viewer.podStatus')} value={session?.pod_status ?? '-'} />
					<SessionDetailRow label={t('viewer.viewerUrl')} value={session?.viewer_url || '-'} />
					<SessionDetailRow label={t('viewer.viewerMode')} value={session?.mode ?? '-'} />
					<SessionDetailRow label={t('viewer.lastHeartbeat')} value={session?.last_heartbeat_at || '-'} />
					{session?.reason
						? <SessionDetailRow label={t('viewer.scheduling')} value={session.reason} />
						: null}
					{sessionCapability.error
						? (
								<div className="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-destructive">
									{translateViewerError(sessionCapability.error, t)}
								</div>
							)
						: null}
					<div className="flex flex-wrap items-center gap-2 pt-1">
						{canRetry
							? (
									<Button onClick={onRefreshSession} size="sm" variant="outline">
										<RefreshCw data-icon="inline-start" />
										{t('actions.retry')}
									</Button>
								)
							: null}
						<SessionActions
							api={api}
							canDiscardLocalState={sessionCapability.kind === 'failed' || sessionCapability.kind === 'manual-closed'}
							onManualClose={onManualClose}
							podSessionID={podSessionID}
							showPodAction={false}
							viewerSessionID={viewerSessionID}
						/>
					</div>
				</div>
			</PopoverContent>
		</Popover>
	)
}

interface SessionDetailRowProps {
	label: string
	value: string
}

function SessionDetailRow({ label, value }: SessionDetailRowProps) {
	return (
		<div className="flex flex-col gap-1">
			<span className="text-xs text-muted-foreground">{label}</span>
			<span className="min-w-0 break-words font-mono text-xs text-foreground">{value}</span>
		</div>
	)
}

interface FileListErrorStateProps {
	api: ViewerAPI
	error: Error
	onManualClose?: (kind: ManualCloseKind) => void
	onRetry: () => void
	podSessionID: string | null
	sessionCapability: SessionCapability
	viewerSessionID: string | null
}

function FileListErrorState({
	api,
	error,
	onManualClose,
	onRetry,
	podSessionID,
	sessionCapability,
	viewerSessionID,
}: FileListErrorStateProps) {
	const { t } = useTranslation()

	return (
		<div className="mx-auto flex max-w-lg flex-col items-center gap-4 text-center">
			<div className="flex size-10 items-center justify-center rounded-full bg-destructive/10 text-destructive">
				<Info />
			</div>
			<div className="flex flex-col gap-1">
				<div className="font-medium text-foreground">{t('files.fileListUnavailable')}</div>
				<div className="text-sm text-muted-foreground">{t(sessionCapability.messageKey)}</div>
				<div className="text-sm text-destructive">
					{error instanceof Error ? error.message : t('errors.generic')}
				</div>
			</div>
			<div className="flex flex-wrap justify-center gap-2">
				<Button onClick={onRetry} size="sm" variant="outline">
					<RefreshCw data-icon="inline-start" />
					{t('actions.retry')}
				</Button>
				<SessionActions
					api={api}
					onManualClose={onManualClose}
					podSessionID={podSessionID}
					showPodAction={false}
					viewerSessionID={viewerSessionID}
				/>
			</div>
		</div>
	)
}

function sessionStatusDotClassName(kind: SessionCapability['kind']) {
	if (kind === 'viewer-ready') {
		return 'bg-emerald-500'
	}
	if (kind === 'failed') {
		return 'bg-destructive'
	}
	if (kind === 'starting-pod' || kind === 'pod-only' || kind === 'viewer-reconnecting') {
		return 'bg-amber-500'
	}
	return 'bg-muted-foreground'
}

interface StorageUsageSummaryProps {
	query: UseQueryResult<FileUsage, Error>
}

function StorageUsageSummary({ query }: StorageUsageSummaryProps) {
	const { t } = useTranslation()
	const usage = query.data
	const percent = usage && usage.total > 0
		? Math.min(100, Math.max(0, Math.round((usage.used / usage.total) * 100)))
		: 0

	if (query.isLoading) {
		return (
			<div className="grid min-w-48 gap-2 rounded-lg border bg-card px-3 py-2" role="status">
				<div className="h-3 w-32 rounded bg-muted" />
				<div className="h-2 w-full rounded bg-muted" />
			</div>
		)
	}

	if (query.error || !usage) {
		return (
			<div className="rounded-lg border bg-card px-3 py-2 text-xs text-muted-foreground">
				{t('files.usageUnavailable')}
			</div>
		)
	}

	return (
		<div className="grid min-w-56 gap-2 rounded-lg border bg-card px-3 py-2">
			<div className="flex items-center justify-between gap-3 text-xs">
				<span className="font-medium text-foreground">{t('files.usedCapacity')}</span>
				<span className="text-muted-foreground">
					{formatBytes(usage.used)}
					{' / '}
					{formatBytes(usage.total)}
				</span>
			</div>
			<Progress aria-label={t('files.usedCapacity')} value={percent} />
		</div>
	)
}

interface UploadTaskRowProps {
	taskID: string
}

function UploadTaskRow({ taskID }: UploadTaskRowProps) {
	const task = useUploadTask(taskID)
	if (!task) {
		return null
	}
	const value = task.bytesTotal > 0
		? Math.round((task.bytesUploaded / task.bytesTotal) * 100)
		: 0

	return (
		<div className="grid gap-1">
			<div className="flex items-center justify-between gap-3 text-xs">
				<span className="truncate">{task.fileName}</span>
				<span className="text-muted-foreground">{task.status}</span>
			</div>
			<Progress
				className={cn(task.status === 'failed' && 'bg-destructive/20 [&_[data-slot=progress-indicator]]:bg-destructive')}
				value={value}
			/>
		</div>
	)
}

function tableColumnClassName(id: string) {
	switch (id) {
		case 'actions':
			return 'w-36 text-right'
		case 'modified':
			return 'w-44'
		case 'size':
			return 'w-28'
		default:
			return 'min-w-0'
	}
}

interface SortableHeadProps {
	active: boolean
	direction: 'asc' | 'desc'
	disabled: boolean
	label: string
	onClick: () => void
}

function SortableHead({ active, direction, disabled, label, onClick }: SortableHeadProps) {
	return (
		<Button disabled={disabled} onClick={onClick} size="sm" variant="ghost">
			{label}
			{active ? <ChevronDown data-icon="inline-end" data-state={direction} /> : null}
		</Button>
	)
}

interface ModifiedTimeCellProps {
	value: string
}

function ModifiedTimeCell({ value }: ModifiedTimeCellProps) {
	const formatted = formatFileModifiedTime(value)
	if (!formatted) {
		return <span>-</span>
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<time className="block truncate text-sm" dateTime={value} title={formatted.long}>
					{formatted.short}
				</time>
			</TooltipTrigger>
			<TooltipContent>
				<time dateTime={value}>{formatted.long}</time>
			</TooltipContent>
		</Tooltip>
	)
}

interface FileNameCellProps {
	disabled: boolean
	isExpanded: boolean
	isLoading: boolean
	onOpen: (entry: FileEntry) => void
	onRetryBranch: (path: string) => void
	onToggleFolder: (entry: FileEntry) => void
	row: FileTableRow
}

function FileNameCell({
	disabled,
	isExpanded,
	isLoading,
	onOpen,
	onRetryBranch,
	onToggleFolder,
	row,
}: FileNameCellProps) {
	const { t } = useTranslation()

	if (row.kind === 'branch-error') {
		return (
			<div className="flex min-w-0 items-center gap-2 text-destructive" style={{ paddingLeft: `${row.depth * 16}px` }}>
				<span>{row.error.message}</span>
				<Button disabled={disabled} onClick={() => onRetryBranch(row.path)} size="sm" variant="outline">
					{t('files.retryFolder')}
				</Button>
			</div>
		)
	}

	const entry = row.entry
	const canOpen = entry.isDir || isEditableFile(entry.path)
	return (
		<div className="flex w-full min-w-0 items-center gap-2" style={{ paddingLeft: `${entry.depth * 16}px` }}>
			{entry.isDir
				? (
						<Button
							aria-label={t('files.toggleFolder')}
							disabled={disabled}
							onClick={() => onToggleFolder(entry)}
							size="icon"
							variant="ghost"
						>
							{isLoading ? <Loader2 className="animate-spin" /> : isExpanded ? <ChevronDown /> : <ChevronRight />}
						</Button>
					)
				: <span className="size-9" />}
			<div className="flex size-8 items-center justify-center rounded-md border bg-muted text-muted-foreground">
				{entry.isDir ? <Folder /> : <File />}
			</div>
			{canOpen
				? (
						<button
							className="group min-w-0 flex-1 cursor-pointer text-left disabled:cursor-not-allowed disabled:opacity-50"
							disabled={disabled}
							onClick={() => onOpen(entry)}
							type="button"
						>
							<span className="block truncate font-medium group-hover:underline">{entry.name}</span>
							<span className="block truncate font-mono text-xs text-muted-foreground group-hover:underline">{entry.path}</span>
						</button>
					)
				: (
						<div className="min-w-0 flex-1">
							<div className="truncate font-medium">{entry.name}</div>
							<div className="truncate font-mono text-xs text-muted-foreground">{entry.path}</div>
						</div>
					)}
		</div>
	)
}

interface FileActionsProps {
	disabled: boolean
	entry: FileEntry
	onOpenFolder: (path: string) => void
	session: FileBrowserSession
}

function FileActions({ disabled, entry, onOpenFolder, session }: FileActionsProps) {
	const { t } = useTranslation()
	const queryClient = useQueryClient()
	const [editing, setEditing] = useState(false)
	const [deleting, setDeleting] = useState(false)
	const deleteMutation = useMutation(moveToRecycleBinMutationOptions(queryClient, session))

	const deleteFile = useCallback(() => {
		deleteMutation.mutate({
			isDir: entry.isDir,
			path: entry.path,
			size: entry.size,
		}, {
			onSuccess: () => {
				toast.success(t('trash.moved'))
				setDeleting(false)
			},
			onError: error => toast.error(error instanceof Error ? error.message : t('errors.generic')),
		})
	}, [deleteMutation, entry.isDir, entry.path, entry.size, t])

	function openEditor() {
		if (entry.size > maxEditableFileBytes) {
			toast.error(t('files.editorTooLarge', { size: formatBytes(maxEditableFileBytes) }))
			return
		}
		setEditing(true)
	}

	const canEdit = !entry.isDir && isEditableFile(entry.path)

	return (
		<>
			<div className="flex justify-end gap-1">
				{entry.isDir
					? (
							<Button
								aria-label={t('files.openFolder')}
								disabled={disabled}
								onClick={() => onOpenFolder(entry.path)}
								size="icon"
								variant="ghost"
							>
								<ChevronRight />
							</Button>
						)
					: (
							<Button aria-label={t('files.download')} disabled={disabled} onClick={() => downloadEntry(session, entry)} size="icon" variant="ghost">
								<Download />
							</Button>
						)}
				{canEdit
					? (
							<Button aria-label={t('files.edit')} disabled={disabled} onClick={openEditor} size="icon" variant="ghost">
								<Edit3 />
							</Button>
						)
					: null}
				<Button aria-label={t('actions.delete')} disabled={disabled} onClick={() => setDeleting(true)} size="icon" variant="ghost">
					<Trash2 />
				</Button>
			</div>

			<FileEditorDialog
				entry={entry}
				onOpenChange={setEditing}
				open={editing}
				session={session}
			/>

			<Dialog onOpenChange={setDeleting} open={deleting}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t('files.confirmDeleteTitle')}</DialogTitle>
						<DialogDescription>{t('files.confirmDeleteDescription', { name: entry.name })}</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button onClick={() => setDeleting(false)} variant="outline">
							{t('actions.cancel')}
						</Button>
						<Button
							disabled={deleteMutation.isPending}
							onClick={deleteFile}
							variant="destructive"
						>
							{t('actions.delete')}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	)
}

interface FileEditorDialogProps {
	entry: FileEntry
	onOpenChange: (open: boolean) => void
	open: boolean
	session: FileBrowserSession
}

function FileEditorDialog({ entry, onOpenChange, open, session }: FileEditorDialogProps) {
	const { t } = useTranslation()
	const queryClient = useQueryClient()
	const textQuery = useQuery(fileTextQueryOptions(session, open ? entry.path : null))
	const [editorState, setEditorState] = useState(() => ({
		content: '',
		dirty: false,
		path: entry.path,
	}))
	const saveMutation = useMutation(saveFileTextMutationOptions(queryClient, session))
	const isCurrentEditorState = editorState.path === entry.path
	const editorContent = isCurrentEditorState ? editorState.content : ''
	const isEditorDirty = isCurrentEditorState && editorState.dirty
	const editorValue = isEditorDirty ? editorContent : (textQuery.data ?? editorContent)
	const isSaving = saveMutation.isPending

	const saveFile = useCallback(() => {
		saveMutation.mutate({
			content: editorValue,
			path: entry.path,
		}, {
			onSuccess: () => {
				toast.success(t('files.saved'))
				onOpenChange(false)
			},
			onError: error => toast.error(error instanceof Error ? error.message : t('errors.generic')),
		})
	}, [editorValue, entry.path, onOpenChange, saveMutation, t])

	return (
		<Dialog onOpenChange={nextOpen => !isSaving && onOpenChange(nextOpen)} open={open}>
			<DialogContent className="sm:max-w-4xl" showCloseButton={!isSaving}>
				<DialogHeader>
					<DialogTitle>{t('files.editorTitle')}</DialogTitle>
					<DialogDescription>{entry.name}</DialogDescription>
				</DialogHeader>
				{isSaving
					? (
							<ModalStatus
								description={t('files.savingDescription')}
								title={t('files.savingTitle')}
							/>
						)
					: null}
				<div className="overflow-hidden rounded-md border">
					{!textQuery.isError
						? (
								<Suspense fallback={<div className="p-4 text-sm text-muted-foreground">{t('common.loading')}</div>}>
									<MonacoEditor
										height="28rem"
										language={editorLanguage(entry.path)}
										loading={t('common.loading')}
										onChange={(value) => {
											setEditorState({
												content: value ?? '',
												dirty: true,
												path: entry.path,
											})
										}}
										options={{
											fontSize: 13,
											minimap: { enabled: false },
											readOnly: isSaving || textQuery.isLoading,
											scrollBeyondLastLine: false,
											wordWrap: 'on',
										}}
										value={textQuery.isLoading ? '' : editorValue}
									/>
								</Suspense>
							)
						: (
								<div className="p-4 text-sm text-destructive">
									{textQuery.error instanceof Error ? textQuery.error.message : t('errors.generic')}
								</div>
							)}
				</div>
				<DialogFooter>
					<Button disabled={isSaving} onClick={() => onOpenChange(false)} type="button" variant="outline">
						{t('actions.cancel')}
					</Button>
					<Button
						disabled={isSaving || textQuery.isLoading || textQuery.isError}
						onClick={saveFile}
						type="button"
					>
						{t('actions.save')}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}

interface DialogWithSessionProps {
	currentPath: string
	disabled: boolean
	podSessionID?: string | null
	session: FileBrowserSession | null
	viewerSessionID?: string | null
}

function CreateFolderDialog({ currentPath, disabled, session }: DialogWithSessionProps) {
	const { t } = useTranslation()
	const queryClient = useQueryClient()
	const [open, setOpen] = useState(false)
	const [name, setName] = useState('')
	const mutation = useMutation(createFolderMutationOptions(queryClient, session))

	const createFolder = useCallback(() => {
		mutation.mutate({
			currentPath,
			name,
		}, {
			onSuccess: () => {
				toast.success(t('files.folderCreated'))
				setName('')
				setOpen(false)
			},
			onError: error => toast.error(error instanceof Error ? error.message : t('errors.generic')),
		})
	}, [currentPath, mutation, name, t])

	return (
		<>
			<Button disabled={disabled} onClick={() => setOpen(true)} size="sm" variant="outline">
				<FolderPlus data-icon="inline-start" />
				{t('files.newFolder')}
			</Button>
			<Dialog onOpenChange={setOpen} open={open}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t('files.newFolder')}</DialogTitle>
						<DialogDescription>{currentPath}</DialogDescription>
					</DialogHeader>
					<div className="grid gap-2">
						<Label htmlFor="folder-name">{t('files.folderName')}</Label>
						<Input
							id="folder-name"
							onChange={event => setName(event.target.value)}
							value={name}
						/>
					</div>
					<DialogFooter>
						<Button onClick={() => setOpen(false)} variant="outline">
							{t('actions.cancel')}
						</Button>
						<Button
							disabled={mutation.isPending || name.trim().length === 0}
							onClick={createFolder}
						>
							{t('actions.create')}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	)
}

function UploadDialog({
	currentPath,
	disabled,
	podSessionID,
	session,
	viewerSessionID,
}: DialogWithSessionProps) {
	const { t } = useTranslation()
	const queryClient = useQueryClient()
	const [open, setOpen] = useState(false)
	const [file, setFile] = useState<File | null>(null)
	const [targetPath, setTargetPath] = useState(currentPath)
	const [activeTaskID, setActiveTaskID] = useState<string | null>(null)
	const inputRef = useRef<HTMLInputElement | null>(null)
	const mutation = useMutation(uploadFileMutationOptions(queryClient, session))
	const activeTask = useUploadTask(activeTaskID)
	const isUploading = mutation.isPending
	const uploadProgress = activeTask && activeTask.bytesTotal > 0
		? Math.round((activeTask.bytesUploaded / activeTask.bytesTotal) * 100)
		: 0
	const chunkLabel = activeTask?.chunkTotal
		? t('files.uploadChunks', {
				current: activeTask.chunkIndex ?? 0,
				total: activeTask.chunkTotal,
			})
		: t('files.uploadPreparing')

	const resetDialogState = useCallback(() => {
		setFile(null)
		setTargetPath(currentPath)
		setActiveTaskID(null)
		if (inputRef.current) {
			inputRef.current.value = ''
		}
	}, [currentPath])

	const openUploadDialog = useCallback(() => {
		resetDialogState()
		setOpen(true)
	}, [resetDialogState])

	const uploadFile = useCallback(() => {
		if (!file) {
			return
		}
		const taskID = createUploadTaskID(file.name)
		setActiveTaskID(taskID)
		mutation.mutate({
			currentPath: targetPath,
			file,
			podSessionID: podSessionID ?? undefined,
			taskID,
			viewerSessionID: viewerSessionID ?? undefined,
		}, {
			onSuccess: () => {
				toast.success(t('files.uploaded'))
				resetDialogState()
				setOpen(false)
			},
			onError: error => toast.error(error instanceof Error ? error.message : t('errors.generic')),
		})
	}, [file, mutation, podSessionID, resetDialogState, t, targetPath, viewerSessionID])

	return (
		<>
			<Button disabled={disabled} onClick={openUploadDialog} size="sm">
				<Upload data-icon="inline-start" />
				{t('files.upload')}
			</Button>
			<Dialog
				onOpenChange={(nextOpen) => {
					if (isUploading) {
						return
					}
					setOpen(nextOpen)
					if (!nextOpen) {
						resetDialogState()
					}
				}}
				open={open}
			>
				<DialogContent showCloseButton={!isUploading}>
					<DialogHeader>
						<DialogTitle>{t('files.upload')}</DialogTitle>
						<DialogDescription>{t('files.uploadTargetDescription')}</DialogDescription>
					</DialogHeader>
					{isUploading
						? (
								<ModalStatus
									description={t('files.uploadingDescription')}
									title={t('files.uploadingTitle')}
								/>
							)
						: null}
					<input
						className="hidden"
						disabled={isUploading}
						onChange={event => setFile(event.target.files?.[0] ?? null)}
						ref={inputRef}
						type="file"
					/>
					<div className="grid gap-3">
						<UploadPathPicker
							disabled={isUploading}
							onTargetPathChange={setTargetPath}
							session={session}
							targetPath={targetPath}
						/>
						<Button disabled={isUploading} onClick={() => inputRef.current?.click()} type="button" variant="outline">
							{t('files.chooseFile')}
						</Button>
						{file
							? (
									<div className="grid gap-2 rounded-md border bg-muted px-3 py-2 text-sm">
										<div>
											{file.name}
											<span className="ml-2 text-muted-foreground">{formatBytes(file.size)}</span>
										</div>
										{isUploading || activeTask?.status === 'failed'
											? (
													<div className="grid gap-1">
														<Progress
															className={cn(activeTask?.status === 'failed' && 'bg-destructive/20 [&_[data-slot=progress-indicator]]:bg-destructive')}
															value={uploadProgress}
														/>
														<div className="flex justify-between gap-3 text-xs text-muted-foreground">
															<span>{activeTask?.status === 'failed' ? t('status.failed') : chunkLabel}</span>
															<span>
																{formatBytes(activeTask?.bytesUploaded ?? 0)}
																{' / '}
																{formatBytes(activeTask?.bytesTotal ?? file.size)}
															</span>
														</div>
														{activeTask?.errorMessage
															? <div className="text-xs text-destructive">{activeTask.errorMessage}</div>
															: null}
													</div>
												)
											: null}
									</div>
								)
							: null}
					</div>
					<DialogFooter>
						<Button disabled={isUploading} onClick={() => setOpen(false)} variant="outline">
							{t('actions.cancel')}
						</Button>
						<Button disabled={!file || isUploading} onClick={uploadFile}>
							{t('files.upload')}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	)
}

interface UploadPathPickerProps {
	disabled: boolean
	onTargetPathChange: (path: string) => void
	session: FileBrowserSession | null
	targetPath: string
}

function UploadPathPicker({
	disabled,
	onTargetPathChange,
	session,
	targetPath,
}: UploadPathPickerProps) {
	const { t } = useTranslation()
	const [browsePath, setBrowsePath] = useState(targetPath)
	const folderQuery = useQuery(fileListQueryOptions(
		session,
		browsePath,
		{ field: 'name', direction: 'asc' },
		!disabled,
	))
	const directories = folderQuery.data?.entries.filter(entry => entry.isDir) ?? emptyEntries

	const choosePath = useCallback((path: string) => {
		setBrowsePath(path)
		onTargetPathChange(path)
	}, [onTargetPathChange])

	return (
		<div className="grid gap-2">
			<Label>{t('files.uploadTarget')}</Label>
			<div className="rounded-md border">
				<div className="flex items-center gap-2 border-b px-3 py-2">
					<Button
						aria-label={t('files.up')}
						disabled={disabled || browsePath === '/'}
						onClick={() => choosePath(parentPath(browsePath))}
						size="icon"
						type="button"
						variant="ghost"
					>
						<ArrowLeft />
					</Button>
					<button
						className="min-w-0 flex-1 rounded-sm px-2 py-1 text-left font-mono text-xs hover:bg-muted disabled:pointer-events-none disabled:opacity-60"
						disabled={disabled}
						onClick={() => onTargetPathChange(browsePath)}
						type="button"
					>
						<span className="block truncate">{targetPath}</span>
					</button>
				</div>
				<div className="max-h-40 overflow-auto p-1">
					{folderQuery.isLoading || folderQuery.isFetching
						? (
								<div className="flex items-center gap-2 px-2 py-2 text-sm text-muted-foreground">
									<Loader2 className="size-4 animate-spin" />
									<span>{t('files.pending')}</span>
								</div>
							)
						: null}
					{folderQuery.error
						? (
								<div className="px-2 py-2 text-sm text-destructive">
									{folderQuery.error instanceof Error ? folderQuery.error.message : t('errors.generic')}
								</div>
							)
						: null}
					{!folderQuery.isLoading && !folderQuery.isFetching && !folderQuery.error && directories.length === 0
						? (
								<div className="px-2 py-2 text-sm text-muted-foreground">{t('files.noFolders')}</div>
							)
						: null}
					{directories.map(entry => (
						<button
							className={cn(
								'flex w-full min-w-0 items-center gap-2 rounded-sm px-2 py-2 text-left text-sm hover:bg-muted disabled:pointer-events-none disabled:opacity-60',
								targetPath === entry.path && 'bg-muted',
							)}
							disabled={disabled}
							key={entry.path}
							onClick={() => choosePath(entry.path)}
							type="button"
						>
							<Folder className="size-4 shrink-0 text-muted-foreground" />
							<span className="truncate">{entry.name}</span>
						</button>
					))}
				</div>
			</div>
		</div>
	)
}

interface ModalStatusProps {
	description: string
	title: string
}

function ModalStatus({ description, title }: ModalStatusProps) {
	return (
		<div className="rounded-md border bg-muted px-3 py-2 text-sm" role="status">
			<div className="font-medium">{title}</div>
			<div className="text-muted-foreground">{description}</div>
		</div>
	)
}

function editorLanguage(path: string) {
	const extension = path.split('.').pop()?.toLowerCase()
	switch (extension) {
		case 'css':
			return 'css'
		case 'html':
			return 'html'
		case 'js':
		case 'jsx':
			return 'javascript'
		case 'json':
			return 'json'
		case 'md':
			return 'markdown'
		case 'ts':
		case 'tsx':
			return 'typescript'
		case 'xml':
			return 'xml'
		case 'yaml':
		case 'yml':
			return 'yaml'
		default:
			return 'plaintext'
	}
}

function formatFileModifiedTime(value: string) {
	if (!value) {
		return null
	}
	const date = new Date(value)
	if (Number.isNaN(date.getTime())) {
		return null
	}
	return {
		long: new Intl.DateTimeFormat(undefined, {
			dateStyle: 'full',
			timeStyle: 'long',
		}).format(date),
		short: new Intl.DateTimeFormat(undefined, {
			dateStyle: 'medium',
			timeStyle: 'short',
		}).format(date),
	}
}

function downloadEntry(session: FileBrowserSession, entry: FileEntry) {
	const anchor = document.createElement('a')
	anchor.href = session.client.downloadUrl(entry.path)
	anchor.download = entry.name
	anchor.rel = 'noreferrer'
	anchor.click()
}

function hasPendingBranches(branches: Record<string, BranchState | undefined>) {
	return Object.values(branches).some(branch => branch?.isLoading)
}
