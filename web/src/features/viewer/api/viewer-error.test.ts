import { APIError, ErrCode } from '@sealos-storage-manager/encore-client'
import { describe, expect, it } from 'vitest'

import {
	formatViewerErrorToast,
	normalizeViewerError,
	translateViewerError,
	ViewerApiError,
	viewerErrorMessageKey,
	viewerErrorMessageKeys,
} from '@/features/viewer/api/viewer-error'
import { backendViewerErrorCodes } from '@/features/viewer/types/viewer'
import { createI18nInstance, resources } from '@/i18n'

describe('viewer errors', () => {
	it('maps every backend viewer error code to a translation key', () => {
		expect(Object.keys(viewerErrorMessageKeys).sort()).toEqual([...backendViewerErrorCodes].sort())
		for (const code of backendViewerErrorCodes) {
			expect(viewerErrorMessageKey(code)).not.toBe('errors.generic')
		}
	})

	it('has zh and en translations for every backend error code', () => {
		for (const code of backendViewerErrorCodes) {
			const key = viewerErrorMessageKey(code)
			expect(resources.en.translation).toHaveProperty(key)
			expect(resources.zh.translation).toHaveProperty(key)
		}
	})

	it('translates known errors and falls back for unknown codes', () => {
		const instance = createI18nInstance('en')
		const known = translateViewerError(
			new ViewerApiError({
				code: 'PVC_ACCESS_DENIED',
				message: 'denied',
				status: 403,
			}),
			instance.t,
		)

		expect(known).toContain('permission')
		expect(instance.t('errors.generic')).toBe(resources.en.translation.errors.generic)
	})

	it('appends backend detail messages to localized errors', () => {
		const instance = createI18nInstance('en')
		const message = 'creating pod ns-admin/viewer-ps-quota: pods "viewer-ps-quota" is forbidden: exceeded quota: quota-ns-admin, requested: pods=1, used: pods=8, limited: pods=8'

		const translated = translateViewerError(
			new APIError(403, {
				code: ErrCode.PermissionDenied,
				details: {
					code: 'VIEWER_POD_FAILED',
					message,
				},
				message: 'viewer pod failed',
			}),
			instance.t,
		)

		expect(translated).toContain('The viewer pod failed to start.')
		expect(translated).toContain('exceeded quota')
	})

	it('formats toast errors with localized summary and backend details', () => {
		const instance = createI18nInstance('en')
		const message = 'persistentvolumeclaims "cache-data" is forbidden: exceeded quota: quota-ns-admin, requested: requests.storage=20Gi, used: requests.storage=90Gi, limited: requests.storage=100Gi'

		const formatted = formatViewerErrorToast(
			new APIError(403, {
				code: ErrCode.PermissionDenied,
				details: {
					code: 'PVC_CREATE_FORBIDDEN',
					message,
				},
				message: 'pvc create forbidden',
			}),
			instance.t,
		)

		expect(formatted.message).toBe('You do not have permission to create PVCs in this namespace.')
		expect(formatted.description).toBe(message)
	})

	it('formats structured backend details when no detail message is available', () => {
		const instance = createI18nInstance('en')

		const formatted = formatViewerErrorToast(
			new ViewerApiError({
				code: 'STORAGE_CLASS_IN_USE',
				details: {
					pvc_count: 2,
					storage_class: 'standard',
				},
				message: 'StorageClass is in use',
				status: 409,
			}),
			instance.t,
		)

		expect(formatted.message).toBe('This StorageClass is used by existing PVCs and cannot be deleted.')
		expect(formatted.description).toContain('pvc_count: 2')
		expect(formatted.description).toContain('storage_class: standard')
	})

	it('omits duplicate toast descriptions when localized copy already includes the detail', () => {
		const instance = createI18nInstance('en')
		const formatted = formatViewerErrorToast(
			new ViewerApiError({
				code: 'VIEWER_POD_FAILED',
				details: {},
				message: 'ImagePullBackOff',
				status: 500,
			}),
			instance.t,
		)

		expect(formatted.message).toBe('The viewer pod failed to start. Reason: ImagePullBackOff')
		expect(formatted.description).toBeUndefined()
	})

	it('uses pod session copy for pod session loss', () => {
		const instance = createI18nInstance('zh')

		const translated = translateViewerError(
			new ViewerApiError({
				code: 'POD_SESSION_NOT_FOUND',
				message: 'Pod session no longer exists',
				status: 404,
			}),
			instance.t,
		)

		expect(translated).toContain('Pod Session 已丢失')
		expect(translated).not.toContain('Viewer Session 已丢失')
	})

	it('normalizes unknown backend codes to a typed internal error', () => {
		const error = normalizeViewerError(new Error('network failed'))

		expect(error.code).toBe('INTERNAL_ERROR')
		expect(viewerErrorMessageKey(error.code)).toBe('errors.internalError')
	})
})
