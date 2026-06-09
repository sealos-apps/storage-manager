import type { UseMutationResult } from '@tanstack/react-query'
import type React from 'react'

import type { StorageClass, ViewerAPI } from '@/features/viewer/types/viewer'
import { useQuery } from '@tanstack/react-query'
import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatViewerErrorToast } from '@/features/viewer/api/viewer-error'
import { adminStorageClassDescribeQueryOptions, adminStorageClassYAMLQueryOptions } from '@/features/viewer/api/viewer-query-options'

const MonacoEditor = lazy(() => import('@monaco-editor/react'))
const largeEditorDialogClassName = 'h-[88vh] max-h-[88vh] w-[min(96vw,90rem)] sm:max-w-[min(96vw,90rem)]'
const storageClassAccessModes = ['ReadWriteOnce', 'ReadOnlyMany', 'ReadWriteMany'] as const

type StorageClassEditorState
	= | { mode: 'create' }
		| { mode: 'edit', name: string }
		| null

interface StorageClassEditorDialogProps {
	api: ViewerAPI
	createMutation: UseMutationResult<StorageClass, Error, { yaml: string }>
	editor: StorageClassEditorState
	onOpenChange: (state: StorageClassEditorState) => void
	updateMutation: UseMutationResult<StorageClass, Error, { name: string, yaml: string }>
}

export function StorageClassEditorDialog({
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
				onError: error => showViewerErrorToast(error, t),
			})
			return
		}
		updateMutation.mutate({ name: editor.name, yaml: yamlBody }, {
			onSuccess: () => {
				toast.success(t('storageClasses.saved'))
				onOpenChange(null)
			},
			onError: error => showViewerErrorToast(error, t),
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

export function StorageClassDescribeDialog({
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

export function StorageClassPolicyDialog({
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
			onError: error => showViewerErrorToast(error, t),
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

export function DeleteStorageClassDialog({
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
								onError: error => showViewerErrorToast(error, t),
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

function defaultStorageClassYAML() {
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: example
  labels:
    app.kubernetes.io/managed-by: sealos-storage-manager
  annotations:
    storage-management.sealos.io/visible-in-create: "true"
    storage-management.sealos.io/access-modes: "ReadWriteOnce"
provisioner: example.com/provisioner
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
`
}

function showViewerErrorToast(error: unknown, t: ReturnType<typeof useTranslation>['t']) {
	const formatted = formatViewerErrorToast(error, t)
	toast.error(formatted.message, { description: formatted.description })
}
