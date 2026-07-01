import type { UseMutationResult } from '@tanstack/react-query'
import type { AdminNamespace, PVC, StorageClass, StorageQuota } from '@/features/viewer/types/viewer'
import type { Quantity } from '@/utils/quantities'

import { useForm } from '@tanstack/react-form'
import { Trans, useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

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
import { capacityBytesToGiInput, formatQuantity, parseGiQuantityInput } from '@/features/viewer/utils/storage-quantity'

interface CreatePVCForm {
	capacityGi: string
	name: string
}

const defaultCreatePVCForm: CreatePVCForm = {
	capacityGi: '10',
	name: '',
}

const pvcAccessModes = ['ReadWriteOnce', 'ReadOnlyMany', 'ReadWriteMany'] as const
interface CreatePVCVariables {
	accessModes: string[]
	capacity: Quantity
	name: string
	namespace: string
	storageClassName: string
}

interface ExpandPVCVariables {
	capacity: Quantity
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
	const { i18n, t } = useTranslation()
	const firstStorageClass = storageClasses[0]
	const targetNamespace = namespaceOptions.length > 0 ? selectedNamespace : namespace
	const defaultStorageClassName = firstStorageClass?.name ?? ''
	const form = useForm({
		defaultValues: {
			accessMode: pvcAccessModes[0] as string,
			...defaultCreatePVCForm,
			storageClassName: defaultStorageClassName,
		},
		onSubmit: ({ value }) => {
			const storageClassName = storageClasses.some(storageClass => storageClass.name === value.storageClassName)
				? value.storageClassName
				: defaultStorageClassName
			const accessMode = (pvcAccessModes as readonly string[]).includes(value.accessMode)
				? value.accessMode
				: pvcAccessModes[0]
			const parsed = createPVCSchema(t).safeParse({ ...value, accessMode, storageClassName })
			if (!parsed.success) {
				toast.error(parsed.error.issues[0]?.message ?? t('errors.validationError'))
				return
			}
			if (exceedsStorageQuota(parsed.data.capacity, storageQuota)) {
				return
			}
			mutation.mutate({
				namespace: targetNamespace,
				name: parsed.data.name,
				capacity: parsed.data.capacity,
				accessModes: [parsed.data.accessMode],
				storageClassName: parsed.data.storageClassName,
			}, {
				onSuccess: () => {
					toast.success(t('volumes.created'))
					form.reset({
						accessMode: pvcAccessModes[0],
						...defaultCreatePVCForm,
						storageClassName: defaultStorageClassName,
					})
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
						<form.Field name="storageClassName">
							{field => (
								<Select
									onValueChange={field.handleChange}
									value={storageClasses.some(storageClass => storageClass.name === field.state.value) ? field.state.value : defaultStorageClassName}
								>
									<SelectTrigger id="pvc-storage-class">
										<SelectValue placeholder={t('volumes.storageClass')} />
									</SelectTrigger>
									<SelectContent>
										<SelectGroup>
											{storageClasses.map(storageClass => (
												<SelectItem key={storageClass.name} value={storageClass.name}>
													{storageClassDisplayName(storageClass, i18n.language)}
													{storageClass.is_default ? ` · ${t('common.default')}` : ''}
												</SelectItem>
											))}
										</SelectGroup>
									</SelectContent>
								</Select>
							)}
						</form.Field>
					</FormField>
					<FormField id="pvc-access-mode" label={t('viewer.accessModes')}>
						<form.Field name="accessMode">
							{field => (
								<Select onValueChange={field.handleChange} value={(pvcAccessModes as readonly string[]).includes(field.state.value) ? field.state.value : pvcAccessModes[0]}>
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
							)}
						</form.Field>
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
										|| parseGiQuantityInput(values.capacityGi) === null
										|| exceedsStorageQuota(parseGiQuantityInput(values.capacityGi), storageQuota)
										|| !(storageClasses.some(storageClass => storageClass.name === values.storageClassName) ? values.storageClassName : defaultStorageClassName)
										|| !((pvcAccessModes as readonly string[]).includes(values.accessMode) ? values.accessMode : pvcAccessModes[0])
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
	const parsed = parseGiQuantityInput(field.state.value)
	const quotaError = parsed !== null && exceedsStorageQuota(parsed, storageQuota)
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
	const currentGi = pvc ? Number(capacityBytesToGiInput(pvc.capacity)) : 1
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
	const form = useForm({
		defaultValues: {
			capacityGi: String(currentGi + 10),
		},
		onSubmit: ({ value }) => {
			if (!pvc) {
				return
			}
			const capacity = parseGiQuantityInput(value.capacityGi)
			const current = parseGiQuantityInput(String(currentGi))
			const delta = capacity !== null && current !== null ? capacity.sub(current) : null
			if (capacity === null || current === null || capacity.cmp(current) <= 0 || exceedsStorageQuota(delta, storageQuota)) {
				return
			}
			mutation.mutate({
				namespace: pvc.namespace,
				name: pvc.name,
				capacity,
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
		},
	})

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
					<DialogDescription className="break-all pr-8">{pvc ? `${pvc.namespace}/${pvc.name}` : ''}</DialogDescription>
				</DialogHeader>
				<form
					className="grid gap-4"
					onSubmit={(event) => {
						event.preventDefault()
						void form.handleSubmit()
					}}
				>
					<div className="flex justify-between gap-4 text-sm">
						<span>{t('volumes.currentCapacity')}</span>
						<span>
							{currentGi}
							{' '}
							Gi
						</span>
					</div>
					<form.Subscribe selector={state => expandCapacityError(state.values.capacityGi, currentGi, storageQuota, t)}>
						{error => (
							<FormField error={error} id="expand-capacity" label={t('volumes.targetCapacity')}>
								<form.Field name="capacityGi">
									{field => (
										<CapacityGiInput
											aria-invalid={error ? true : undefined}
											id="expand-capacity"
											onBlur={() => field.handleChange(normalizeCapacityGi(field.state.value))}
											onChange={event => field.handleChange(event.target.value)}
											value={field.state.value}
										/>
									)}
								</form.Field>
							</FormField>
						)}
					</form.Subscribe>
					<p className="text-sm text-muted-foreground">{t('volumes.expandHint')}</p>
					<DialogFooter>
						<Button onClick={() => onOpenChange(null)} type="button" variant="outline">
							{t('actions.cancel')}
						</Button>
						<form.Subscribe selector={state => expandCapacityError(state.values.capacityGi, currentGi, storageQuota, t)}>
							{error => (
								<Button disabled={!pvc || mutation.isPending || Boolean(error)} type="submit">
									{t('volumes.expand')}
								</Button>
							)}
						</form.Subscribe>
					</DialogFooter>
				</form>
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
	const form = useForm({
		defaultValues: {
			confirmName: deleteState?.confirmName ?? '',
		},
		onSubmit: ({ value }) => {
			if (!pvc || !deleteConfirmSchema(pvc.name).safeParse(value.confirmName).success) {
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
		},
	})
	return (
		<Dialog onOpenChange={open => !open && onOpenChange(null)} open={deleteState !== null}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t('volumes.deleteTitle')}</DialogTitle>
					<DialogDescription>
						{pvc
							? (
									<Trans
										components={{
											name: <strong className="select-all font-semibold text-foreground" />,
										}}
										i18nKey="volumes.deleteDescription"
										values={{ name: pvc.name }}
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
					{pvc
						? (
								<form.Field name="confirmName">
									{field => (
										<FormField id="delete-confirm" label={t('volumes.typeNameToConfirm')}>
											<Input
												id="delete-confirm"
												onChange={event => field.handleChange(event.target.value)}
												value={field.state.value}
											/>
										</FormField>
									)}
								</form.Field>
							)
						: null}
					<DialogFooter>
						<Button onClick={() => onOpenChange(null)} type="button" variant="outline">
							{t('actions.cancel')}
						</Button>
						<form.Subscribe selector={state => state.values.confirmName}>
							{confirmName => (
								<Button disabled={!pvc || mutation.isPending || confirmName !== pvc.name} type="submit" variant="destructive">
									{t('actions.delete')}
								</Button>
							)}
						</form.Subscribe>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

function createPVCSchema(t: ReturnType<typeof useTranslation>['t']) {
	return z.object({
		accessMode: z.enum(pvcAccessModes),
		capacityGi: z.string().transform((value, ctx) => {
			const quantity = parseGiQuantityInput(value)
			if (quantity === null) {
				ctx.addIssue({ code: z.ZodIssueCode.custom, message: t('volumes.capacityRequired') })
				return z.NEVER
			}
			return quantity
		}),
		name: z.string().trim().min(1, t('volumes.nameRequired')),
		storageClassName: z.string().trim().min(1, t('volumes.noAvailableStorageClasses')),
	}).transform(value => ({ ...value, capacity: value.capacityGi }))
}

function parseCapacityGi(value: string) {
	const quantity = parseGiQuantityInput(value)
	return quantity === null ? null : Number(value.trim())
}

function normalizeCapacityGi(value: string) {
	const parsed = parseCapacityGi(value)
	return parsed === null ? value : String(parsed)
}

function exceedsStorageQuota(required: Quantity | null, storageQuota: StorageQuota | null) {
	return storageQuota !== null && required !== null && required.cmp(storageQuota.available) > 0
}

function storageQuotaError(t: ReturnType<typeof useTranslation>['t'], storageQuota: StorageQuota | null) {
	return t('volumes.storageQuotaAvailable', {
		quantity: storageQuota ? formatQuantity(storageQuota.available) : '0',
	})
}

function expandCapacityError(
	value: string,
	currentGi: number,
	storageQuota: StorageQuota | null,
	t: ReturnType<typeof useTranslation>['t'],
) {
	const quantity = parseGiQuantityInput(value)
	const currentQuantity = parseGiQuantityInput(String(currentGi))
	const requestedDelta = quantity !== null && currentQuantity !== null ? quantity.sub(currentQuantity) : null
	if (quantity !== null && currentQuantity !== null && quantity.cmp(currentQuantity) > 0) {
		return exceedsStorageQuota(requestedDelta, storageQuota) ? storageQuotaError(t, storageQuota) : ''
	}
	return t('volumes.capacityRequired')
}

function deleteConfirmSchema(name: string) {
	return z.literal(name)
}

function storageClassDisplayName(storageClass: StorageClass, locale: string) {
	const names = storageClass.display_names ?? {}
	return names[locale] ?? names[locale.split('-')[0] ?? ''] ?? storageClass.name
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
