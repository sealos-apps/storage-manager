import type { FileBrowserSession, FileEntry, FileTableRow } from '@/features/file-manager/types/file-manager'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, Download, Edit3, File, Folder, Loader2, Trash2 } from 'lucide-react'
import { useCallback, useState } from 'react'
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
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { moveToRecycleBinMutationOptions } from '@/features/file-manager/api/file-manager-mutations'
import { FileEditorDialog } from '@/features/file-manager/components/file-dialogs'
import { downloadEntry, formatFileModifiedTime } from '@/features/file-manager/utils/file-manager-format'
import { isEditableFile } from '@/features/file-manager/utils/file-tree'
import { formatBytes } from '@/features/viewer/utils/format-capacity'

const maxEditableFileBytes = 32 * 1024 * 1024

interface SortableHeadProps {
	active: boolean
	direction: 'asc' | 'desc'
	disabled: boolean
	label: string
	onClick: () => void
}

export function SortableHead({ active, direction, disabled, label, onClick }: SortableHeadProps) {
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

export function ModifiedTimeCell({ value }: ModifiedTimeCellProps) {
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

export function FileNameCell({
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

export function FileActions({ disabled, entry, onOpenFolder, session }: FileActionsProps) {
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
							<Button aria-label={t('files.download')} disabled={disabled} onClick={() => void downloadEntry(session, entry).catch(error => toast.error(error instanceof Error ? error.message : t('errors.generic')))} size="icon" variant="ghost">
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
