import type { UseMutationResult } from '@tanstack/react-query'
import type React from 'react'

import type { PVC, StorageClass, ViewerAPI } from '@/features/viewer/types/viewer'
import { useForm } from '@tanstack/react-form'
import { useQuery } from '@tanstack/react-query'
import { lazy, Suspense, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

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
import { adminStorageClassDescribeQueryOptions, adminStorageClassYAMLQueryOptions, pvcDescribeQueryOptions, pvcYAMLQueryOptions } from '@/features/viewer/api/viewer-query-options'

const MonacoEditor = lazy(() => import('@/components/monaco-editor'))
const largeEditorDialogClassName = 'h-[88vh] max-h-[88vh] w-[min(96vw,90rem)] sm:max-w-[min(96vw,90rem)]'

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
	const yamlSchema = z.string().trim().min(1, t('storageClasses.yamlRequired'))
	const form = useForm({
		defaultValues: {
			yaml: '',
		},
		onSubmit: () => {
			if (!editor) {
				return
			}
			const parsed = yamlSchema.safeParse(yamlBody)
			if (!parsed.success) {
				toast.error(parsed.error.issues[0]?.message ?? t('storageClasses.yamlRequired'))
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
		},
	})

	return (
		<Dialog onOpenChange={nextOpen => !isPending && onOpenChange(nextOpen ? editor : null)} open={open}>
			<DialogContent className={largeEditorDialogClassName}>
				<DialogHeader>
					<DialogTitle>{isCreate ? t('storageClasses.create') : t('storageClasses.yaml')}</DialogTitle>
					<DialogDescription>{isCreate ? t('storageClasses.createDescription') : name}</DialogDescription>
				</DialogHeader>
				<form
					className="contents"
					onSubmit={(event) => {
						event.preventDefault()
						void form.handleSubmit()
					}}
				>
					<div className="min-h-0 overflow-hidden rounded-md border">
						<Suspense fallback={<div className="p-4 text-sm text-muted-foreground">{t('common.loading')}</div>}>
							<form.Field name="yaml">
								{field => (
									<MonacoEditor
										height="calc(88vh - 12rem)"
										language="yaml"
										loading={t('common.loading')}
										onChange={(value) => {
											const nextValue = value ?? ''
											field.handleChange(nextValue)
											setYamlDraft({ key: editorKey, value: nextValue })
										}}
										options={{
											fontSize: 13,
											minimap: { enabled: false },
											readOnly: isPending || yamlQuery.isLoading,
											scrollBeyondLastLine: false,
											wordWrap: 'on',
										}}
										value={yamlQuery.isLoading && !isCreate ? '' : yamlBody}
									/>
								)}
							</form.Field>
						</Suspense>
					</div>
					<DialogFooter>
						<Button disabled={isPending} onClick={() => onOpenChange(null)} type="button" variant="outline">
							{t('actions.cancel')}
						</Button>
						<Button disabled={isPending || (!isCreate && yamlQuery.isLoading)} type="submit">
							{t('actions.save')}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

export function StorageClassMetadataDialog({
	mutation,
	onOpenChange,
	storageClass,
}: {
	mutation: UseMutationResult<StorageClass, Error, { availableToUsers: boolean, displayNames: Record<string, string>, name: string }>
	onOpenChange: (name: string | null) => void
	storageClass: StorageClass | null
}) {
	const { t } = useTranslation()
	const form = useForm({
		defaultValues: {
			availableToUsers: storageClass?.available_to_users ?? false,
			displayNames: displayNameRows(storageClass),
		},
		onSubmit: ({ value }) => {
			if (!storageClass) {
				return
			}
			const parsed = displayNamesSchema(t).safeParse(value.displayNames)
			if (!parsed.success) {
				toast.error(parsed.error.issues[0]?.message ?? t('storageClasses.displayNameInvalid'))
				return
			}
			mutation.mutate({
				name: storageClass.name,
				availableToUsers: value.availableToUsers,
				displayNames: Object.fromEntries(parsed.data.map(row => [row.locale.trim(), row.name.trim()])),
			}, {
				onSuccess: () => {
					toast.success(t('storageClasses.metadataSaved'))
					onOpenChange(null)
				},
				onError: error => showViewerErrorToast(error, t),
			})
		},
	})

	return (
		<Dialog onOpenChange={open => onOpenChange(open ? storageClass?.name ?? null : null)} open={Boolean(storageClass)}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t('storageClasses.editMetadata')}</DialogTitle>
					<DialogDescription>{storageClass?.name ?? ''}</DialogDescription>
				</DialogHeader>
				<form
					className="grid gap-4"
					onSubmit={(event) => {
						event.preventDefault()
						void form.handleSubmit()
					}}
				>
					<form.Field name="availableToUsers">
						{field => (
							<label className="flex items-center gap-2 text-sm">
								<Checkbox
									checked={field.state.value}
									onCheckedChange={value => field.handleChange(value === true)}
								/>
								{t('storageClasses.availableToUsers')}
							</label>
						)}
					</form.Field>
					<div className="grid gap-2">
						<form.Field name="displayNames">
							{field => (
								<>
									<div className="flex items-center justify-between gap-3">
										<Label>{t('storageClasses.displayNames')}</Label>
										<Button
											onClick={() => field.handleChange([...field.state.value, { id: newRowID(), locale: '', name: '' }])}
											size="sm"
											type="button"
											variant="outline"
										>
											{t('storageClasses.addDisplayName')}
										</Button>
									</div>
									<div className="grid gap-2">
										{field.state.value.map((row, index) => (
											<div className="grid grid-cols-[7rem_1fr_auto] gap-2" key={row.id}>
												<Input
													aria-label={t('storageClasses.locale')}
													onChange={event => field.handleChange(field.state.value.map((item, rowIndex) => rowIndex === index ? { ...item, locale: event.target.value } : item))}
													placeholder="zh"
													value={row.locale}
												/>
												<Input
													aria-label={t('storageClasses.displayName')}
													onChange={event => field.handleChange(field.state.value.map((item, rowIndex) => rowIndex === index ? { ...item, name: event.target.value } : item))}
													value={row.name}
												/>
												<Button
													onClick={() => field.handleChange(field.state.value.filter((_, rowIndex) => rowIndex !== index))}
													type="button"
													variant="outline"
												>
													{t('actions.delete')}
												</Button>
											</div>
										))}
									</div>
								</>
							)}
						</form.Field>
					</div>
					<DialogFooter>
						<Button onClick={() => onOpenChange(null)} type="button" variant="outline">
							{t('actions.cancel')}
						</Button>
						<Button disabled={mutation.isPending} type="submit">
							{t('actions.save')}
						</Button>
					</DialogFooter>
				</form>
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

export function PVCYAMLDialog({
	api,
	mutation,
	onOpenChange,
	pvc,
}: {
	api: ViewerAPI
	mutation: UseMutationResult<PVC, Error, { name: string, namespace: string, yaml: string }>
	onOpenChange: (pvc: PVC | null) => void
	pvc: PVC | null
}) {
	const { t } = useTranslation()
	const yamlQuery = useQuery(pvcYAMLQueryOptions(pvc, api))
	const [yamlDraft, setYamlDraft] = useState({ key: '', value: '' })
	const key = pvc ? `${pvc.namespace}/${pvc.name}` : ''
	const yamlBody = yamlDraft.key === key ? yamlDraft.value : yamlQuery.data?.yaml ?? ''
	const yamlSchema = z.string().trim().min(1, t('storageClasses.yamlRequired'))
	const form = useForm({
		defaultValues: {
			yaml: '',
		},
		onSubmit: () => {
			if (!pvc) {
				return
			}
			const parsed = yamlSchema.safeParse(yamlBody)
			if (!parsed.success) {
				toast.error(parsed.error.issues[0]?.message ?? t('storageClasses.yamlRequired'))
				return
			}
			mutation.mutate({ namespace: pvc.namespace, name: pvc.name, yaml: yamlBody }, {
				onSuccess: () => {
					toast.success(t('volumes.saved'))
					onOpenChange(null)
				},
				onError: error => showViewerErrorToast(error, t),
			})
		},
	})

	return (
		<Dialog onOpenChange={open => onOpenChange(open ? pvc : null)} open={Boolean(pvc)}>
			<DialogContent className={largeEditorDialogClassName}>
				<DialogHeader>
					<DialogTitle>{t('storageClasses.yaml')}</DialogTitle>
					<DialogDescription>{pvc ? `${pvc.namespace}/${pvc.name}` : ''}</DialogDescription>
				</DialogHeader>
				<form
					className="contents"
					onSubmit={(event) => {
						event.preventDefault()
						void form.handleSubmit()
					}}
				>
					<div className="min-h-0 overflow-hidden rounded-md border">
						<Suspense fallback={<div className="p-4 text-sm text-muted-foreground">{t('common.loading')}</div>}>
							<form.Field name="yaml">
								{field => (
									<MonacoEditor
										height="calc(88vh - 12rem)"
										language="yaml"
										loading={t('common.loading')}
										onChange={(value) => {
											const nextValue = value ?? ''
											field.handleChange(nextValue)
											setYamlDraft({ key, value: nextValue })
										}}
										options={{
											fontSize: 13,
											minimap: { enabled: false },
											readOnly: mutation.isPending || yamlQuery.isLoading,
											scrollBeyondLastLine: false,
											wordWrap: 'on',
										}}
										value={yamlQuery.isLoading ? '' : yamlBody}
									/>
								)}
							</form.Field>
						</Suspense>
					</div>
					<DialogFooter>
						<Button disabled={mutation.isPending} onClick={() => onOpenChange(null)} type="button" variant="outline">
							{t('actions.cancel')}
						</Button>
						<Button disabled={!pvc || mutation.isPending || yamlQuery.isLoading} type="submit">
							{t('actions.save')}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

export function PVCDescribeDialog({
	api,
	onOpenChange,
	pvc,
}: {
	api: ViewerAPI
	onOpenChange: (pvc: PVC | null) => void
	pvc: PVC | null
}) {
	const { t } = useTranslation()
	const describeQuery = useQuery(pvcDescribeQueryOptions(pvc, api))

	return (
		<Dialog onOpenChange={open => onOpenChange(open ? pvc : null)} open={Boolean(pvc)}>
			<DialogContent className={largeEditorDialogClassName}>
				<DialogHeader>
					<DialogTitle>{t('storageClasses.describe')}</DialogTitle>
					<DialogDescription>{pvc ? `${pvc.namespace}/${pvc.name}` : ''}</DialogDescription>
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
	const form = useForm({
		defaultValues: {
			confirmName: '',
		},
		onSubmit: () => {
			if (!name || !z.literal(name).safeParse(confirmName).success) {
				return
			}
			mutation.mutate(name, {
				onSuccess: () => {
					toast.success(t('storageClasses.deleted'))
					onOpenChange(null)
				},
				onError: error => showViewerErrorToast(error, t),
			})
		},
	})

	return (
		<Dialog onOpenChange={open => onOpenChange(open ? name : null)} open={Boolean(name)}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t('storageClasses.deleteTitle')}</DialogTitle>
					<DialogDescription>
						{name
							? (
									<Trans
										components={{
											name: <strong className="select-all font-semibold text-foreground" />,
										}}
										i18nKey="storageClasses.deleteDescription"
										values={{ name }}
									/>
								)
							: null}
					</DialogDescription>
				</DialogHeader>
				<form
					className="grid gap-4"
					onSubmit={(event) => {
						event.preventDefault()
						void form.handleSubmit()
					}}
				>
					<form.Field name="confirmName">
						{field => (
							<FormField id="delete-storage-class-confirm" label={t('storageClasses.typeNameToConfirm')}>
								<Input
									id="delete-storage-class-confirm"
									onChange={(event) => {
										field.handleChange(event.target.value)
										setConfirmState({ name, value: event.target.value })
									}}
									value={confirmName}
								/>
							</FormField>
						)}
					</form.Field>
					<DialogFooter>
						<Button onClick={() => onOpenChange(null)} type="button" variant="outline">
							{t('actions.cancel')}
						</Button>
						<Button disabled={!name || confirmName !== name || mutation.isPending} type="submit" variant="destructive">
							{t('actions.delete')}
						</Button>
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

function displayNameRows(storageClass: StorageClass | null) {
	return Object.entries(storageClass?.display_names ?? {})
		.sort(([left], [right]) => left.localeCompare(right))
		.map(([locale, name]) => ({ id: newRowID(), locale, name }))
}

function newRowID() {
	return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`
}

function displayNamesSchema(t: ReturnType<typeof useTranslation>['t']) {
	return z.array(z.object({
		locale: z.string().trim().min(1, t('storageClasses.localeRequired')),
		name: z.string().trim().min(1, t('storageClasses.displayNameRequired')),
	})).superRefine((rows, ctx) => {
		const seen = new Set<string>()
		for (const [index, row] of rows.entries()) {
			const locale = row.locale.trim()
			if (seen.has(locale)) {
				ctx.addIssue({
					code: z.ZodIssueCode.custom,
					message: t('storageClasses.localeDuplicate'),
					path: [index, 'locale'],
				})
			}
			seen.add(locale)
		}
	})
}

function defaultStorageClassYAML() {
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: example
  labels:
    app.kubernetes.io/managed-by: sealos-storage-manager
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
