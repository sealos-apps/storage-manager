import type { FileBrowserSession, FileEntry } from '@/features/file-manager/types/file-manager'

import { parentPath } from '@sealos-storage-manager/filebrowser-client'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Folder, FolderPlus, Loader2, Upload } from 'lucide-react'
import { lazy, Suspense, useCallback, useRef, useState } from 'react'
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
import { Progress } from '@/components/ui/progress'
import {
	createFolderMutationOptions,
	createUploadTaskID,
	saveFileTextMutationOptions,
	uploadFileMutationOptions,
} from '@/features/file-manager/api/file-manager-mutations'
import { fileListQueryOptions, fileTextQueryOptions } from '@/features/file-manager/api/file-manager-query-options'
import { useUploadTask } from '@/features/file-manager/stores/upload-store'
import { editorLanguage } from '@/features/file-manager/utils/file-manager-format'
import { formatBytes } from '@/features/viewer/utils/format-capacity'
import { cn } from '@/utils/cn'

const MonacoEditor = lazy(() => import('@/components/monaco-editor'))
const largeEditorDialogClassName = 'h-[88vh] max-h-[88vh] w-[min(96vw,90rem)] sm:max-w-[min(96vw,90rem)]'
const emptyEntries: FileEntry[] = []

interface FileEditorDialogProps {
	entry: FileEntry
	onOpenChange: (open: boolean) => void
	open: boolean
	session: FileBrowserSession
}

export function FileEditorDialog({ entry, onOpenChange, open, session }: FileEditorDialogProps) {
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
			<DialogContent className={largeEditorDialogClassName} showCloseButton={!isSaving}>
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
				<div className="min-h-0 overflow-hidden rounded-md border">
					{!textQuery.isError
						? (
								<Suspense fallback={<div className="p-4 text-sm text-muted-foreground">{t('common.loading')}</div>}>
									<MonacoEditor
										height="calc(88vh - 12rem)"
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

export function CreateFolderDialog({ currentPath, disabled, session }: DialogWithSessionProps) {
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

export function UploadDialog({
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
