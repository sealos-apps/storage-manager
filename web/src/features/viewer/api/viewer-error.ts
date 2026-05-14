import type { TFunction } from 'i18next'
import type { ViewerApiErrorShape, ViewerErrorCode } from '@/features/viewer/types/viewer'

import { isAPIError } from '@/services/encore/client'

export const viewerErrorMessageKeys = {
	PVC_NOT_FOUND: 'errors.pvcNotFound',
	PVC_ACCESS_DENIED: 'errors.pvcAccessDenied',
	UNSUPPORTED_ACCESS_MODE: 'errors.unsupportedAccessMode',
	PVC_MOUNT_CONFLICT: 'errors.pvcMountConflict',
	PVC_MOUNT_PENDING: 'errors.pvcMountPending',
	VIEWER_POD_CREATING: 'errors.viewerPodCreating',
	VIEWER_POD_FAILED: 'errors.viewerPodFailed',
	VIEWER_SESSION_NOT_FOUND: 'errors.viewerSessionNotFound',
	VIEWER_SESSION_EXPIRED: 'errors.viewerSessionExpired',
	AUTH_REQUEST_EXPIRED: 'errors.authRequestExpired',
	AUTH_REQUEST_USED: 'errors.authRequestUsed',
	FILEBROWSER_LOGIN_FAILED: 'errors.fileBrowserLoginFailed',
	HOOK_VERIFY_FAILED: 'errors.hookVerifyFailed',
	UNAUTHORIZED: 'errors.unauthorized',
	VALIDATION_ERROR: 'errors.validationError',
	INTERNAL_ERROR: 'errors.internalError',
} as const satisfies Record<ViewerErrorCode, string>

interface EncoreErrorDetails {
	Code?: string
	code?: string
}

export class ViewerApiError extends Error implements ViewerApiErrorShape {
	readonly code: string
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

export function normalizeViewerError(error: unknown): ViewerApiError {
	if (error instanceof ViewerApiError) {
		return error
	}
	if (isAPIError(error)) {
		return new ViewerApiError({
			code: detailCode(error.details) ?? error.code,
			details: typeof error.details === 'object' && error.details !== null
				? error.details as Record<string, unknown>
				: {},
			message: error.message,
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

export function viewerErrorMessageKey(code: string) {
	return viewerErrorMessageKeys[code as ViewerErrorCode] ?? 'errors.generic'
}

export function translateViewerError(error: unknown, t: TFunction) {
	const apiError = normalizeViewerError(error)
	return t(viewerErrorMessageKey(apiError.code), {
		defaultValue: t('errors.generic'),
		reason: apiError.message,
	})
}
