import type { UseMutationResult } from '@tanstack/react-query'
import type { PVC, StorageClass } from '@/features/viewer/types/viewer'

import { useForm } from '@tanstack/react-form'
import { useMemo, useState } from 'react'
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
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { translateViewerError } from '@/features/viewer/api/viewer-error'

interface CreatePVCForm {
	capacityGi: number
	name: string
}

const defaultCreatePVCForm: CreatePVCForm = {
	capacityGi: 10,
	name: '',
}

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

export interface DeletePVCState {
	confirmName: string
	pvc: PVC
}

interface CreatePVCDialogProps {
	mutation: UseMutationResult<PVC, Error, CreatePVCVariables>
	namespace: string
	onOpenChange: (open: boolean) => void
	open: boolean
	storageClasses: StorageClass[]
}

export function CreatePVCDialog({
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

export function ExpandPVCDialog({ mutation, onOpenChange, pvc }: ExpandPVCDialogProps) {
	const { t } = useTranslation()
	const currentGi = pvc ? Math.max(1, Math.ceil(pvc.capacity_bytes / 1024 / 1024 / 1024)) : 1
	return (
		<ExpandPVCDialogContent
			currentGi={currentGi}
			mutation={mutation}
			onOpenChange={onOpenChange}
			pvc={pvc}
			key={pvc?.uid ?? 'closed'}
			t={t}
		/>
	)
}

function ExpandPVCDialogContent({
	currentGi,
	mutation,
	onOpenChange,
	pvc,
	t,
}: ExpandPVCDialogProps & {
	currentGi: number
	t: ReturnType<typeof useTranslation>['t']
}) {
	const [nextGi, setNextGi] = useState(currentGi + 10)
	const value = Number.isFinite(nextGi) ? Math.floor(nextGi) : 0
	const capacityError = value > currentGi ? '' : t('volumes.capacityRequired')

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
					<FormField error={capacityError} id="expand-capacity" label={t('volumes.targetCapacity')}>
						<Input
							aria-invalid={capacityError ? true : undefined}
							id="expand-capacity"
							min={currentGi + 1}
							onChange={event => setNextGi(Number(event.target.value))}
							step={1}
							type="number"
							value={Number.isFinite(nextGi) ? nextGi : ''}
						/>
					</FormField>
					<p className="text-sm text-muted-foreground">{t('volumes.expandHint')}</p>
				</div>
				<DialogFooter>
					<Button onClick={() => onOpenChange(null)} variant="outline">
						{t('actions.cancel')}
					</Button>
					<Button
						disabled={!pvc || mutation.isPending || Boolean(capacityError)}
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

export function DeletePVCDialog({
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
