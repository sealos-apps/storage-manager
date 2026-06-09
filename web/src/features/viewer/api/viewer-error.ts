import type { TFunction } from 'i18next'
import type { ViewerApiErrorShape, ViewerErrorCode } from '@/features/viewer/types/viewer'

import { isAPIError } from '@sealos-storage-manager/encore-client'
import { backendViewerErrorCodes } from '@/features/viewer/types/viewer'

export const viewerErrorMessageKeys = {
	PVC_NOT_FOUND: 'errors.pvcNotFound',
	PVC_ALREADY_EXISTS: 'errors.pvcAlreadyExists',
	PVC_IN_USE: 'errors.pvcInUse',
	PVC_ACCESS_DENIED: 'errors.pvcAccessDenied',
	PVC_CREATE_FORBIDDEN: 'errors.pvcCreateForbidden',
	PVC_DELETE_FORBIDDEN: 'errors.pvcDeleteForbidden',
	PVC_EXPAND_FORBIDDEN: 'errors.pvcExpandForbidden',
	PVC_EXPAND_UNSUPPORTED: 'errors.pvcExpandUnsupported',
	PVC_EXPAND_NOT_INCREASED: 'errors.pvcExpandNotIncreased',
	UNSUPPORTED_ACCESS_MODE: 'errors.unsupportedAccessMode',
	PVC_MOUNT_CONFLICT: 'errors.pvcMountConflict',
	PVC_MOUNT_PENDING: 'errors.pvcMountPending',
	STORAGE_CLASS_NOT_FOUND: 'errors.storageClassNotFound',
	STORAGE_CLASS_NOT_VISIBLE: 'errors.storageClassNotVisible',
	STORAGE_CLASS_YAML_INVALID: 'errors.storageClassYAMLInvalid',
	STORAGE_CLASS_CONFLICT: 'errors.storageClassConflict',
	STORAGE_CLASS_DELETE_FORBIDDEN: 'errors.storageClassDeleteForbidden',
	STORAGE_CLASS_IN_USE: 'errors.storageClassInUse',
	ADMIN_ACCESS_DENIED: 'errors.adminAccessDenied',
	VIEWER_POD_CREATING: 'errors.viewerPodCreating',
	VIEWER_POD_FAILED: 'errors.viewerPodFailed',
	POD_SESSION_NOT_FOUND: 'errors.podSessionNotFound',
	VIEWER_SESSION_NOT_FOUND: 'errors.viewerSessionNotFound',
	VIEWER_SESSION_EXPIRED: 'errors.viewerSessionExpired',
	AUTH_REQUEST_EXPIRED: 'errors.authRequestExpired',
	AUTH_REQUEST_USED: 'errors.authRequestUsed',
	FILEBROWSER_LOGIN_FAILED: 'errors.fileBrowserLoginFailed',
	FILE_MANAGEMENT_DISABLED: 'errors.fileManagementDisabled',
	HOOK_VERIFY_FAILED: 'errors.hookVerifyFailed',
	UNAUTHORIZED: 'errors.unauthorized',
	VALIDATION_ERROR: 'errors.validationError',
	INTERNAL_ERROR: 'errors.internalError',
} as const satisfies Record<ViewerErrorCode, string>

interface EncoreErrorDetails {
	Code?: string
	message?: string
	code?: string
	Message?: string
}

const backendViewerErrorCodeSet = new Set<string>(backendViewerErrorCodes)

export class ViewerApiError extends Error implements ViewerApiErrorShape {
	readonly code: ViewerErrorCode
	readonly details: Record<string, unknown>
	readonly status?: number

	constructor({ code, details, message, status }: ViewerApiErrorShape) {
		super(message)
		this.name = 'ViewerApiError'
		this.code = code
		this.details = details ?? {}
		this.status = status
	}
}

function detailCode(details: unknown): string | undefined {
	if (!details || typeof details !== 'object') {
		return undefined
	}
	const typed = details as EncoreErrorDetails
	return typed.Code ?? typed.code
}

function detailMessage(details: unknown): string | undefined {
	if (!details || typeof details !== 'object') {
		return undefined
	}
	const typed = details as EncoreErrorDetails
	return typed.Message ?? typed.message
}

export function isViewerErrorCode(code: string): code is ViewerErrorCode {
	return backendViewerErrorCodeSet.has(code)
}

function normalizeViewerErrorCode(code: string | undefined): ViewerErrorCode {
	if (code && isViewerErrorCode(code)) {
		return code
	}
	return 'INTERNAL_ERROR'
}

export function normalizeViewerError(error: unknown): ViewerApiError {
	if (error instanceof ViewerApiError) {
		return error
	}
	if (isAPIError(error)) {
		const message = detailMessage(error.details) ?? error.message
		return new ViewerApiError({
			code: normalizeViewerErrorCode(detailCode(error.details) ?? error.code),
			details: typeof error.details === 'object' && error.details !== null
				? error.details as Record<string, unknown>
				: {},
			message,
			status: error.status,
		})
	}
	if (error instanceof Error) {
		return new ViewerApiError({
			code: 'INTERNAL_ERROR',
			message: error.message,
		})
	}
	return new ViewerApiError({
		code: 'INTERNAL_ERROR',
		message: 'Unknown viewer error',
	})
}

export function isViewerApiError(error: unknown): error is ViewerApiError {
	return error instanceof ViewerApiError
}

export function isMissingSessionError(error: unknown) {
	const apiError = normalizeViewerError(error)
	return apiError.code === 'VIEWER_SESSION_NOT_FOUND' || apiError.code === 'POD_SESSION_NOT_FOUND'
}

export function viewerErrorMessageKey(code: ViewerErrorCode) {
	return viewerErrorMessageKeys[code]
}

export function translateViewerError(error: unknown, t: TFunction) {
	const apiError = normalizeViewerError(error)
	const localized = t(viewerErrorMessageKey(apiError.code), {
		defaultValue: t('errors.generic'),
		reason: apiError.message,
	})
	if (!apiError.message || localized.includes(apiError.message)) {
		return localized
	}
	return `${localized}\n${apiError.message}`
}
