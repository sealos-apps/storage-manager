import { APIError, ErrCode } from '@sealos-storage-manager/encore-client'

import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiTarget, createViewerApi, readAuthorizationHeader } from '@/features/viewer/api/viewer-api'
import { ViewerApiError } from '@/features/viewer/api/viewer-error'
import {
	adminNamespaceFixture,
	pvcFixture,
	storageClassDescribeFixture,
	storageClassFixture,
	storageClassYAMLFixture,
	viewerContextFixture,
	viewerSessionFixture,
} from '@/features/viewer/test/fakes'
import {
	initializeSealosAuthorization,
	resetSealosAuthorizationForTest,
} from '@/services/sealos/sealos-authorization'

describe('viewer API adapter', () => {
	afterEach(() => {
		resetSealosAuthorizationForTest()
		delete window.__SEALOS_STORAGE_MANAGER_CONFIG__
		window.localStorage.clear()
		vi.resetModules()
		vi.unstubAllEnvs()
	})

	it('normalizes configured API base URL for generated endpoint paths', async () => {
		expect(apiTarget()).toBe('/api')

		window.__SEALOS_STORAGE_MANAGER_CONFIG__ = {
			apiBaseUrl: '/api',
		}
		vi.resetModules()
		const sameOriginProxy = await import('@/features/viewer/api/viewer-api')
		expect(sameOriginProxy.apiTarget()).toBe('/api')

		window.__SEALOS_STORAGE_MANAGER_CONFIG__ = {
			apiBaseUrl: 'https://storage.example.com/',
		}
		vi.resetModules()
		const absolute = await import('@/features/viewer/api/viewer-api')
		expect(absolute.apiTarget()).toBe('https://storage.example.com')
	})

	it('calls the public rewrite path by default', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_DEV_KUBECONFIG', 'test-kubeconfig')
		vi.spyOn(console, 'warn').mockImplementation(() => undefined)
		await initializeSealosAuthorization()

		const sameOriginFetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({
			context: viewerContextFixture(),
		})))
		const sameOrigin = createViewerApi(undefined, sameOriginFetcher)
		await expect(sameOrigin.getContext()).resolves.toEqual(expect.objectContaining({
			namespace: 'ns-admin',
		}))
		expect(sameOriginFetcher).toHaveBeenCalledWith('/api/context', expect.objectContaining({
			method: 'GET',
		}))
	})

	it('calls absolute backend roots directly', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_DEV_KUBECONFIG', 'test-kubeconfig')
		vi.stubEnv('VITE_API_BASE_URL', 'https://storage.example.com/')
		vi.resetModules()
		const authorizationModule = await import('@/services/sealos/sealos-authorization')
		const absoluteModule = await import('@/features/viewer/api/viewer-api')
		vi.spyOn(console, 'warn').mockImplementation(() => undefined)
		await authorizationModule.initializeSealosAuthorization()
		const absoluteFetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({
			context: viewerContextFixture(),
		})))
		const absolute = absoluteModule.createViewerApi(undefined, absoluteFetcher)
		await absolute.getContext()
		expect(absoluteFetcher).toHaveBeenCalledWith('https://storage.example.com/context', expect.objectContaining({
			method: 'GET',
		}))
	})

	it('reads authorization from the Sealos bootstrap cache', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_DEV_KUBECONFIG', 'apiVersion: v1\nclusters: []')
		vi.spyOn(console, 'warn').mockImplementation(() => undefined)
		await initializeSealosAuthorization()

		expect(readAuthorizationHeader()).toBe('Bearer apiVersion%3A%20v1%0Aclusters%3A%20%5B%5D')
	})

	it('ignores legacy authorization sources after bootstrap', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_DEV_KUBECONFIG', 'apiVersion: v1\nclusters: []')
		vi.stubEnv('VITE_VIEWER_AUTHORIZATION', 'Bearer configured')
		window.localStorage.setItem('sealos-storage-manager.kubeconfig', 'apiVersion: v1\nclusters: []')
		vi.spyOn(console, 'warn').mockImplementation(() => undefined)
		await initializeSealosAuthorization()

		expect(readAuthorizationHeader()).toBe('Bearer apiVersion%3A%20v1%0Aclusters%3A%20%5B%5D')
	})

	it('throws a localized business error shape when authorization is read before bootstrap', () => {
		expect(() => readAuthorizationHeader()).toThrow(ViewerApiError)
		expect(() => readAuthorizationHeader()).toThrow('Kubeconfig authorization has not been initialized')
	})

	it('unwraps Encore response envelopes through the generated client boundary', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_DEV_KUBECONFIG', 'test-kubeconfig')
		vi.spyOn(console, 'warn').mockImplementation(() => undefined)
		await initializeSealosAuthorization()
		const listPVCs = vi.fn().mockResolvedValue({
			pvc_list: { items: [pvcFixture({ name: 'mysql-data' })] },
		})
		const listStorageClasses = vi.fn().mockResolvedValue({
			storage_class_list: { items: [storageClassFixture({ name: 'standard' })] },
		})
		const adminCapabilities = vi.fn().mockResolvedValue({
			admin_capabilities: { can_manage_pvcs: true, can_manage_storage_classes: true, file_management_enabled: true, user_namespace: 'ns-admin' },
		})
		const adminCreateStorageClass = vi.fn().mockResolvedValue({
			storage_class: storageClassFixture({ name: 'created' }),
		})
		const adminDeleteStorageClass = vi.fn().mockResolvedValue({
			storage_class: storageClassFixture({ name: 'standard' }),
		})
		const adminDescribeStorageClass = vi.fn().mockResolvedValue({
			storage_class_describe: storageClassDescribeFixture({ name: 'standard' }),
		})
		const adminGetStorageClassYAML = vi.fn().mockResolvedValue({
			storage_class_yaml: storageClassYAMLFixture({ name: 'standard' }),
		})
		const adminListNamespaces = vi.fn().mockResolvedValue({
			namespace_list: { items: [adminNamespaceFixture({ name: 'kube-system' })] },
		})
		const adminListStorageClasses = vi.fn().mockResolvedValue({
			storage_class_list: { items: [storageClassFixture({ name: 'admin-standard' })] },
		})
		const adminUpdateStorageClass = vi.fn().mockResolvedValue({
			storage_class: storageClassFixture({ name: 'standard' }),
		})
		const adminUpdateStorageClassPolicy = vi.fn().mockResolvedValue({
			storage_class: storageClassFixture({ name: 'standard', allowed_access_modes: ['ReadWriteMany'] }),
		})
		const createPVC = vi.fn().mockResolvedValue({
			pvc: pvcFixture({ name: 'cache-data' }),
		})
		const expandPVC = vi.fn().mockResolvedValue({
			pvc: pvcFixture({ capacity: '20Gi', capacity_bytes: 20 * 1024 * 1024 * 1024 }),
		})
		const deletePVC = vi.fn().mockResolvedValue({
			pvc: pvcFixture({ name: 'mysql-data' }),
		})
		const getContext = vi.fn().mockResolvedValue({
			context: viewerContextFixture(),
		})
		const createViewerSession = vi.fn().mockResolvedValue({
			viewer_session: viewerSessionFixture({ id: 'vs_1' }),
		})
		const api = createViewerApi({
			viewer: {
				AdminCapabilities: adminCapabilities,
				AdminCreateStorageClass: adminCreateStorageClass,
				AdminDeleteStorageClass: adminDeleteStorageClass,
				AdminDescribeStorageClass: adminDescribeStorageClass,
				AdminGetStorageClassYAML: adminGetStorageClassYAML,
				AdminListNamespaces: adminListNamespaces,
				AdminListStorageClasses: adminListStorageClasses,
				AdminUpdateStorageClass: adminUpdateStorageClass,
				AdminUpdateStorageClassPolicy: adminUpdateStorageClassPolicy,
				ListPVCs: listPVCs,
				ListStorageClasses: listStorageClasses,
				CreatePVC: createPVC,
				CreateViewerSession: createViewerSession,
				ExpandPVC: expandPVC,
				DeletePVC: deletePVC,
				GetContext: getContext,
			},
		} as never)

		await expect(api.getContext()).resolves.toEqual(expect.objectContaining({
			namespace: 'ns-admin',
		}))
		await expect(api.listPVCs({ namespace: 'default' })).resolves.toEqual([
			expect.objectContaining({ name: 'mysql-data' }),
		])
		await expect(api.createViewerSession({
			namespace: 'default',
			pvcName: 'mysql-data',
		})).resolves.toEqual(expect.objectContaining({ id: 'vs_1' }))
		await expect(api.listStorageClasses()).resolves.toEqual([
			expect.objectContaining({ name: 'standard' }),
		])
		await expect(api.adminCapabilities()).resolves.toEqual({ can_manage_pvcs: true, can_manage_storage_classes: true, file_management_enabled: true, user_namespace: 'ns-admin' })
		await expect(api.adminListNamespaces()).resolves.toEqual([
			expect.objectContaining({ name: 'kube-system' }),
		])
		await expect(api.adminListStorageClasses()).resolves.toEqual([
			expect.objectContaining({ name: 'admin-standard' }),
		])
		await expect(api.adminGetStorageClassYAML('standard')).resolves.toEqual(expect.objectContaining({ name: 'standard' }))
		await expect(api.adminDescribeStorageClass('standard')).resolves.toEqual(expect.objectContaining({ name: 'standard' }))
		await expect(api.adminCreateStorageClass({ yaml: 'kind: StorageClass' })).resolves.toEqual(expect.objectContaining({ name: 'created' }))
		await expect(api.adminUpdateStorageClass('standard', { yaml: 'kind: StorageClass' })).resolves.toEqual(expect.objectContaining({ name: 'standard' }))
		await expect(api.adminUpdateStorageClassPolicy('standard', {
			allowedAccessModes: ['ReadWriteMany'],
			visibleInCreate: true,
		})).resolves.toEqual(expect.objectContaining({ name: 'standard' }))
		await expect(api.adminDeleteStorageClass('standard')).resolves.toEqual(expect.objectContaining({ name: 'standard' }))
		await expect(api.createPVC({
			namespace: 'default',
			name: 'cache-data',
			capacity: '5Gi',
			capacityBytes: 5 * 1024 * 1024 * 1024,
			accessModes: ['ReadWriteOnce'],
			storageClassName: 'standard',
		})).resolves.toEqual(expect.objectContaining({ name: 'cache-data' }))
		await expect(api.expandPVC({
			namespace: 'default',
			name: 'mysql-data',
			capacity: '20Gi',
			capacityBytes: 20 * 1024 * 1024 * 1024,
		})).resolves.toEqual(expect.objectContaining({ capacity: '20Gi' }))
		await expect(api.deletePVC({
			namespace: 'default',
			name: 'mysql-data',
		})).resolves.toEqual(expect.objectContaining({ name: 'mysql-data' }))
		expect(listPVCs).toHaveBeenCalledWith({
			Authorization: 'Bearer test-kubeconfig',
			Namespace: 'default',
		})
		expect(createViewerSession).toHaveBeenCalledWith({
			Authorization: 'Bearer test-kubeconfig',
			namespace: 'default',
			pvc_name: 'mysql-data',
		})
		expect(listStorageClasses).toHaveBeenCalledWith({
			Authorization: 'Bearer test-kubeconfig',
		})
		expect(adminCapabilities).toHaveBeenCalledWith({
			Authorization: 'Bearer test-kubeconfig',
		})
		expect(adminListNamespaces).toHaveBeenCalledWith({
			Authorization: 'Bearer test-kubeconfig',
		})
		expect(adminListStorageClasses).toHaveBeenCalledWith({
			Authorization: 'Bearer test-kubeconfig',
		})
		expect(adminGetStorageClassYAML).toHaveBeenCalledWith('standard', {
			Authorization: 'Bearer test-kubeconfig',
		})
		expect(adminDescribeStorageClass).toHaveBeenCalledWith('standard', {
			Authorization: 'Bearer test-kubeconfig',
		})
		expect(adminCreateStorageClass).toHaveBeenCalledWith({
			Authorization: 'Bearer test-kubeconfig',
			yaml: 'kind: StorageClass',
		})
		expect(adminUpdateStorageClass).toHaveBeenCalledWith('standard', {
			Authorization: 'Bearer test-kubeconfig',
			yaml: 'kind: StorageClass',
		})
		expect(adminUpdateStorageClassPolicy).toHaveBeenCalledWith('standard', {
			Authorization: 'Bearer test-kubeconfig',
			allowed_access_modes: ['ReadWriteMany'],
			visible_in_create: true,
		})
		expect(adminDeleteStorageClass).toHaveBeenCalledWith('standard', {
			Authorization: 'Bearer test-kubeconfig',
		})
		expect(createPVC).toHaveBeenCalledWith({
			Authorization: 'Bearer test-kubeconfig',
			namespace: 'default',
			name: 'cache-data',
			capacity: '5Gi',
			capacity_bytes: 5 * 1024 * 1024 * 1024,
			access_modes: ['ReadWriteOnce'],
			storage_class_name: 'standard',
		})
		expect(expandPVC).toHaveBeenCalledWith('default', 'mysql-data', {
			Authorization: 'Bearer test-kubeconfig',
			capacity: '20Gi',
			capacity_bytes: 20 * 1024 * 1024 * 1024,
		})
		expect(deletePVC).toHaveBeenCalledWith('default', 'mysql-data', {
			Authorization: 'Bearer test-kubeconfig',
		})
		expect(getContext).toHaveBeenCalledWith({
			Authorization: 'Bearer test-kubeconfig',
		})
	})

	it('normalizes Encore error detail codes to viewer business errors', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_DEV_KUBECONFIG', 'test-kubeconfig')
		vi.spyOn(console, 'warn').mockImplementation(() => undefined)
		await initializeSealosAuthorization()
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
