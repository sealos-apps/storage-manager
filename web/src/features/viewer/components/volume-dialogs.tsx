import type { UseMutationResult } from '@tanstack/react-query'
import type { AdminNamespace, PVC, StorageClass, StorageQuota } from '@/features/viewer/types/viewer'

import { useForm } from '@tanstack/react-form'
import { useState } from 'react'
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
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
	InputGroupText,
} from '@/components/ui/input-group'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { formatViewerErrorToast } from '@/features/viewer/api/viewer-error'

interface CreatePVCForm {
	capacityGi: string
	name: string
}

const defaultCreatePVCForm: CreatePVCForm = {
	capacityGi: '10',
	name: '',
}

const pvcAccessModes = ['ReadWriteOnce', 'ReadOnlyMany', 'ReadWriteMany'] as const
const bytesPerGi = 1024 * 1024 * 1024

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
	namespaceOptions?: AdminNamespace[]
	onOpenChange: (open: boolean) => void
	onNamespaceChange?: (namespace: string) => void
	open: boolean
	selectedNamespace?: string
	storageClasses: StorageClass[]
	storageQuota: StorageQuota | null
}

export function CreatePVCDialog({
	mutation,
	namespace,
	namespaceOptions = [],
	onOpenChange,
	onNamespaceChange,
	open,
	selectedNamespace = namespace,
	storageClasses,
	storageQuota,
}: CreatePVCDialogProps) {
	const { t } = useTranslation()
	const firstStorageClass = storageClasses[0]
	const [selection, setSelection] = useState({ accessMode: '', storageClassName: '' })
	const targetNamespace = namespaceOptions.length > 0 ? selectedNamespace : namespace
	const activeStorageClassName = storageClasses.some(storageClass => storageClass.name === selection.storageClassName)
		? selection.storageClassName
		: firstStorageClass?.name ?? ''
	const activeAccessMode = (pvcAccessModes as readonly string[]).includes(selection.accessMode)
		? selection.accessMode
		: pvcAccessModes[0]
	const form = useForm({
		defaultValues: {
			...defaultCreatePVCForm,
		},
		onSubmit: ({ value }) => {
			const capacityGi = parseCapacityGi(value.capacityGi)
			if (!activeStorageClassName || !activeAccessMode) {
				return
			}
			if (capacityGi === null) {
				return
			}
			if (exceedsStorageQuota(capacityGi * bytesPerGi, storageQuota)) {
				return
			}
			mutation.mutate({
				namespace: targetNamespace,
				name: value.name.trim(),
				capacity: `${capacityGi}Gi`,
				capacityBytes: capacityGi * bytesPerGi,
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
				onError: (error) => {
					const formatted = formatViewerErrorToast(error, t)
					toast.error(formatted.message, { description: formatted.description })
				},
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
					{namespaceOptions.length > 0
						? (
								<FormField id="pvc-target-namespace" label={t('volumes.targetNamespace')}>
									<Select onValueChange={value => onNamespaceChange?.(value)} value={targetNamespace}>
										<SelectTrigger className="w-full" id="pvc-target-namespace">
											<SelectValue placeholder={t('volumes.targetNamespace')} />
										</SelectTrigger>
										<SelectContent>
											<SelectGroup>
												{namespaceOptions.map(item => (
													<SelectItem key={item.name} value={item.name}>
														{item.name}
														{item.is_current_context ? ` · ${t('common.current')}` : ''}
													</SelectItem>
												))}
											</SelectGroup>
										</SelectContent>
									</Select>
								</FormField>
							)
						: (
								<div className="rounded-md border bg-muted px-3 py-2 text-sm">
									<span className="text-muted-foreground">
										{t('common.namespace')}
										:
										{' '}
									</span>
									<span className="font-medium">{namespace || t('common.loading')}</span>
								</div>
							)}
					<form.Field
						name="capacityGi"
						validators={{
							onBlur: ({ value }) => parseCapacityGi(value) !== null ? undefined : t('volumes.capacityRequired'),
						}}
					>
						{field => (
							<CreateCapacityField
								error={field.state.meta.errorMap.onBlur}
								field={field}
								storageQuota={storageQuota}
								t={t}
							/>
						)}
					</form.Field>
					<FormField id="pvc-storage-class" label={t('volumes.storageClass')}>
						<Select
							onValueChange={(value) => {
								setSelection({
									accessMode: activeAccessMode,
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
									{pvcAccessModes.map(mode => (
										<SelectItem key={mode} value={mode}>{mode}</SelectItem>
									))}
								</SelectGroup>
							</SelectContent>
						</Select>
					</FormField>
					{storageClasses.length === 0
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
										|| !targetNamespace
										|| values.name.trim().length === 0
										|| parseCapacityGi(values.capacityGi) === null
										|| exceedsStorageQuota((parseCapacityGi(values.capacityGi) ?? 0) * bytesPerGi, storageQuota)
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

function CreateCapacityField({
	error,
	field,
	storageQuota,
	t,
}: {
	error?: unknown
	field: {
		handleBlur: () => void
		handleChange: (value: string) => void
		name: string
		state: { value: string }
	}
	storageQuota: StorageQuota | null
	t: ReturnType<typeof useTranslation>['t']
}) {
	const parsed = parseCapacityGi(field.state.value)
	const quotaError = parsed !== null && exceedsStorageQuota(parsed * bytesPerGi, storageQuota)
		? storageQuotaError(t, storageQuota)
		: ''
	const fieldError = quotaError || error

	return (
		<FormField
			error={fieldError}
			id="pvc-capacity"
			label={t('viewer.capacity')}
		>
			<CapacityGiInput
				aria-invalid={fieldError ? true : undefined}
				id="pvc-capacity"
				name={field.name}
				onBlur={() => {
					const normalized = normalizeCapacityGi(field.state.value)
					if (normalized !== field.state.value) {
						field.handleChange(normalized)
					}
					field.handleBlur()
				}}
				onChange={event => field.handleChange(event.target.value)}
				value={field.state.value}
			/>
		</FormField>
	)
}

interface ExpandPVCDialogProps {
	mutation: UseMutationResult<PVC, Error, ExpandPVCVariables>
	onOpenChange: (pvc: PVC | null) => void
	pvc: PVC | null
	storageQuota: StorageQuota | null
}

export function ExpandPVCDialog({ mutation, onOpenChange, pvc, storageQuota }: ExpandPVCDialogProps) {
	const { t } = useTranslation()
	const currentGi = pvc ? Math.max(1, Math.ceil(pvc.capacity_bytes / 1024 / 1024 / 1024)) : 1
	return (
		<ExpandPVCDialogContent
			currentGi={currentGi}
			mutation={mutation}
			onOpenChange={onOpenChange}
			pvc={pvc}
			storageQuota={storageQuota}
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
	storageQuota,
	t,
}: ExpandPVCDialogProps & {
	currentGi: number
	t: ReturnType<typeof useTranslation>['t']
}) {
	const [nextGiInput, setNextGiInput] = useState(String(currentGi + 10))
	const value = parseCapacityGi(nextGiInput)
	const requestedDeltaBytes = value !== null ? (value - currentGi) * bytesPerGi : 0
	const capacityError = value !== null && value > currentGi
		? (exceedsStorageQuota(requestedDeltaBytes, storageQuota) ? storageQuotaError(t, storageQuota) : '')
		: t('volumes.capacityRequired')

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
						<CapacityGiInput
							aria-invalid={capacityError ? true : undefined}
							id="expand-capacity"
							onBlur={() => setNextGiInput(normalizeCapacityGi(nextGiInput))}
							onChange={event => setNextGiInput(event.target.value)}
							value={nextGiInput}
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
							if (value === null) {
								return
							}
							mutation.mutate({
								namespace: pvc.namespace,
								name: pvc.name,
								capacity: `${value}Gi`,
								capacityBytes: value * bytesPerGi,
							}, {
								onSuccess: () => {
									toast.success(t('volumes.expanded'))
									onOpenChange(null)
								},
								onError: (error) => {
									const formatted = formatViewerErrorToast(error, t)
									toast.error(formatted.message, { description: formatted.description })
								},
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
								onError: (error) => {
									const formatted = formatViewerErrorToast(error, t)
									toast.error(formatted.message, { description: formatted.description })
								},
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

function parseCapacityGi(value: string) {
	const trimmed = value.trim()
	if (!/^\d+$/.test(trimmed)) {
		return null
	}
	const parsed = Number(trimmed)
	return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null
}

function normalizeCapacityGi(value: string) {
	const parsed = parseCapacityGi(value)
	return parsed === null ? value : String(parsed)
}

function exceedsStorageQuota(requiredBytes: number, storageQuota: StorageQuota | null) {
	return storageQuota !== null && requiredBytes > storageQuota.available_bytes
}

function storageQuotaError(t: ReturnType<typeof useTranslation>['t'], storageQuota: StorageQuota | null) {
	return t('volumes.storageQuotaAvailable', {
		quantity: storageQuota?.available_quantity ?? '0',
	})
}

function CapacityGiInput({
	className,
	...props
}: React.ComponentProps<typeof InputGroupInput>) {
	return (
		<InputGroup className={className}>
			<InputGroupInput
				inputMode="numeric"
				pattern="[0-9]*"
				{...props}
			/>
			<InputGroupAddon align="inline-end">
				<InputGroupText>Gi</InputGroupText>
			</InputGroupAddon>
		</InputGroup>
	)
}
