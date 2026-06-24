import type { FileBrowserResource } from '@sealos-storage-manager/filebrowser-client'
import type { UseQueryResult } from '@tanstack/react-query'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import type { FileBrowserSession, FileEntry, FileListResult, FileTableRow } from '@/features/file-manager/types/file-manager'
import type { FileSortState } from '@/features/file-manager/utils/file-tree'
import type { PVC, ViewerAPI, ViewerSession } from '@/features/viewer/types/viewer'
import type { ManualCloseKind, SessionCapability } from '@/features/viewer/utils/session-capability'

import { parentPath } from '@sealos-storage-manager/filebrowser-client'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { flexRender, getCoreRowModel, getSortedRowModel, useReactTable } from '@tanstack/react-table'
import {
	ArrowLeft,
	RefreshCw,
} from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
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
import { invalidateFileTreeQueries } from '@/features/file-manager/api/file-manager-cache'
import { fileListQueryOptions } from '@/features/file-manager/api/file-manager-query-options'
import { CreateFolderDialog, FileEditorDialog, UploadDialog } from '@/features/file-manager/components/file-dialogs'
import { FileActions, FileNameCell, ModifiedTimeCell, SortableHead } from '@/features/file-manager/components/file-table-cells'
import { FileListErrorState, SessionStatusPopover } from '@/features/file-manager/components/session-status-popover'
import { UploadTaskList } from '@/features/file-manager/components/upload-task-list'
import { hasPendingBranches } from '@/features/file-manager/utils/file-manager-format'
import { tableColumnClassName } from '@/features/file-manager/utils/file-table'
import {
	buildFileTableRows,
	flattenResources,
	isEditableFile,
	nextSortState,
	sortEntries,
} from '@/features/file-manager/utils/file-tree'
import { viewerApi } from '@/features/viewer/api/viewer-api'
import { formatBytes } from '@/features/viewer/utils/format-capacity'
import { formatQuantity, quantityPercent } from '@/features/viewer/utils/storage-quantity'

interface FileManagerViewProps {
	api?: ViewerAPI
	currentPath: string
	onBackToVolumes: () => void
	onManualClose?: (kind: ManualCloseKind) => void
	onPathChange: (path: string) => void
	onRefreshSession: () => void
	onRefreshStorageData: () => void
	podSessionID?: string | null
	pvc: PVC | null
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
	onRefreshSession,
	onRefreshStorageData,
	podSessionID,
	pvc,
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
					{canShowFileList ? <StorageUsageSummary pvc={pvc} /> : null}
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
											onRefreshStorageData()
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

					<SessionStatusPopover
						api={api}
						onManualClose={onManualClose}
						onRefreshSession={onRefreshSession}
						podSessionID={podSessionID ?? null}
						session={viewerSession ?? null}
						sessionCapability={sessionCapability}
						viewerSessionID={viewerSessionID ?? null}
					/>
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

interface StorageUsageSummaryProps {
	pvc: PVC | null
}

function StorageUsageSummary({ pvc }: StorageUsageSummaryProps) {
	const { t } = useTranslation()
	const stats = pvc?.volume_stats
	const percent = stats && pvc ? quantityPercent(stats.used, pvc.capacity) : 0

	if (!stats) {
		return (
			<div className="rounded-lg border bg-card px-3 py-2 text-xs text-muted-foreground">
				{t('volumes.usageUnavailable')}
			</div>
		)
	}

	if (stats.status !== 'ready') {
		return (
			<div className="rounded-lg border bg-card px-3 py-2 text-xs text-muted-foreground">
				<span className="font-medium text-amber-700">{t('volumes.usageMismatch')}</span>
			</div>
		)
	}

	return (
		<div className="grid min-w-56 gap-1.5 rounded-lg border bg-card px-3 py-2">
			<div className="flex items-center justify-between gap-2 text-xs">
				<span className="font-medium text-foreground">
					{`${formatQuantity(stats.used)} / ${formatQuantity(pvc.capacity)}`}
				</span>
				<span className="text-muted-foreground">
					{`${percent}%`}
				</span>
			</div>
			<Progress aria-label={t('volumes.usageProgressLabel', { pvc: pvc.name })} value={percent} />
			<div className="text-xs text-muted-foreground">
				{t('volumes.usageFree', { size: formatQuantity(stats.available) })}
			</div>
		</div>
	)
}
