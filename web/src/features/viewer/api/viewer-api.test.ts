import { afterEach, describe, expect, it, vi } from 'vitest'

import { createViewerApi, readAuthorizationHeader } from '@/features/viewer/api/viewer-api'
import { ViewerApiError } from '@/features/viewer/api/viewer-error'
import { pvcFixture, viewerSessionFixture } from '@/features/viewer/test/fakes'
import { APIError, ErrCode } from '@/services/encore/client'

describe('viewer API adapter', () => {
	afterEach(() => {
		window.localStorage.clear()
		vi.unstubAllEnvs()
	})

	it('reads configured authorization before local kubeconfig storage', () => {
		vi.stubEnv('VITE_VIEWER_AUTHORIZATION', 'Bearer configured')
		window.localStorage.setItem('sealos-storage-manager.kubeconfig', 'stored')

		expect(readAuthorizationHeader()).toBe('Bearer configured')
	})

	it('encodes stored kubeconfig authorization for the backend auth contract', () => {
		window.localStorage.setItem('sealos-storage-manager.kubeconfig', 'apiVersion: v1\nclusters: []')

		expect(readAuthorizationHeader()).toBe('Bearer apiVersion%3A%20v1%0Aclusters%3A%20%5B%5D')
	})

	it('throws a localized business error shape when authorization is missing', () => {
		expect(() => readAuthorizationHeader()).toThrow(ViewerApiError)
		expect(() => readAuthorizationHeader()).toThrow('Kubeconfig authorization is not configured')
	})

	it('unwraps Encore response envelopes through the generated client boundary', async () => {
		window.localStorage.setItem('sealos-storage-manager.kubeconfig', 'test-kubeconfig')
		const listPVCs = vi.fn().mockResolvedValue({
			pvc_list: { items: [pvcFixture({ name: 'mysql-data' })] },
		})
		const createViewerSession = vi.fn().mockResolvedValue({
			viewer_session: viewerSessionFixture({ id: 'vs_1' }),
		})
		const api = createViewerApi({
			viewer: {
				ListPVCs: listPVCs,
				CreateViewerSession: createViewerSession,
			},
		} as never)

		await expect(api.listPVCs({ namespace: 'default' })).resolves.toEqual([
			expect.objectContaining({ name: 'mysql-data' }),
		])
		await expect(api.createViewerSession({
			namespace: 'default',
			pvcName: 'mysql-data',
		})).resolves.toEqual(expect.objectContaining({ id: 'vs_1' }))
		expect(listPVCs).toHaveBeenCalledWith({
			Authorization: 'Bearer test-kubeconfig',
			Namespace: 'default',
		})
		expect(createViewerSession).toHaveBeenCalledWith({
			Authorization: 'Bearer test-kubeconfig',
			namespace: 'default',
			pvc_name: 'mysql-data',
		})
	})

	it('normalizes Encore error detail codes to viewer business errors', async () => {
		window.localStorage.setItem('sealos-storage-manager.kubeconfig', 'test-kubeconfig')
		const api = createViewerApi({
			viewer: {
				ListPVCs: vi.fn().mockRejectedValue(new APIError(403, {
					code: ErrCode.PermissionDenied,
					details: { Code: 'PVC_ACCESS_DENIED' },
					message: 'denied',
				})),
			},
		} as never)

		await expect(api.listPVCs({ namespace: 'default' })).rejects.toMatchObject({
			code: 'PVC_ACCESS_DENIED',
			status: 403,
		})
	})
})
