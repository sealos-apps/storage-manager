import type { SessionV1 } from '@labring/sealos-desktop-sdk'
import type { SealosAuthorizationState } from '@/services/sealos/sealos-authorization'

import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ALL_NAMESPACES } from '@/features/viewer/api/viewer-constants'
import { ViewerApiError } from '@/features/viewer/api/viewer-error'
import { StorageAppShell } from '@/features/viewer/components/storage-app-shell'
import { viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'
import {
	createFakeViewerAPI,
	pvcFixture,
	storageClassDescribeFixture,
	storageClassFixture,
	storageClassYAMLFixture,
	storageQuotaFixture,
	viewerSessionFixture,
	viewerTokenFixture,
} from '@/features/viewer/test/fakes'
import { renderWithProviders } from '@/test/render'

const sealosAuthorizationMockState = vi.hoisted(() => ({
	authorization: {
		accountAuthorizationHeader: 'Bearer account.jwt.token',
		authorizationHeader: 'Bearer test',
		session: {
			kubeconfig: 'apiVersion: v1\nclusters: []',
			subscription: {} as SessionV1['subscription'],
			user: {
				avatar: '',
				id: 'user-1',
				k8sUsername: 'ns-admin',
				name: 'Admin',
				nsid: 'admin',
			},
		},
		source: 'sdk',
	} as SealosAuthorizationState,
}))

vi.mock('@monaco-editor/react', () => ({
	default: ({
		onChange,
		value,
	}: {
		onChange?: (value?: string) => void
		value?: string
	}) => (
		<textarea
			aria-label="Monaco editor"
			onChange={event => onChange?.(event.target.value)}
			value={value ?? ''}
		/>
	),
}))

vi.mock('@/services/sealos/sealos-authorization', async importOriginal => ({
	...(await importOriginal<typeof import('@/services/sealos/sealos-authorization')>()),
	getCachedSealosAuthorization: vi.fn(() => sealosAuthorizationMockState.authorization),
}))

describe('storageAppShell', () => {
	beforeEach(() => {
		viewerUIStore.actions.reset()
		vi.unstubAllEnvs()
		sealosAuthorizationMockState.authorization = {
			accountAuthorizationHeader: 'Bearer account.jwt.token',
			authorizationHeader: 'Bearer test',
			session: {
				kubeconfig: 'apiVersion: v1\nclusters: []',
				subscription: {} as SessionV1['subscription'],
				user: {
					avatar: '',
					id: 'user-1',
					k8sUsername: 'ns-admin',
					name: 'Admin',
					nsid: 'admin',
				},
			},
			source: 'sdk',
		}
	})

	it('renders PVCs, launches File Browser, and shows real file manager state', async () => {
		const user = userEvent.setup()
		const listPVCs = vi.fn().mockResolvedValue([
			pvcFixture({ name: 'mysql-data', namespace: 'ns-admin', uid: 'uid-1' }),
			pvcFixture({ name: 'logs', namespace: 'ns-admin', uid: 'uid-2' }),
		])
		const api = createFakeViewerAPI({
			createViewerSession: vi.fn().mockResolvedValue(viewerSessionFixture({
				id: 'vs_1',
				status: 'ready',
				token_ready: true,
			})),
			issueViewerToken: vi.fn().mockResolvedValue(viewerTokenFixture({
				viewer_session_id: 'vs_1',
				viewer_url: 'https://viewer.example.test',
			})),
			listPVCs,
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('mysql-data')).toBeInTheDocument()
		expect(screen.getByText('logs')).toBeInTheDocument()
		expect(screen.getByRole('columnheader', { name: /storage class/i })).toBeInTheDocument()
		expect(screen.getAllByText('standard').length).toBeGreaterThan(0)
		expect(screen.queryByRole('columnheader', { name: /capacity/i })).not.toBeInTheDocument()
		expect(screen.queryByText('10Gi')).not.toBeInTheDocument()
		expect(screen.getAllByText('ns-admin').length).toBeGreaterThan(0)
		expect(listPVCs).toHaveBeenCalledWith({ namespace: 'ns-admin' })
		expect(listPVCs).not.toHaveBeenCalledWith({ namespace: 'default' })

		const browseButtons = await screen.findAllByRole('button', { name: /browse files/i })
		const firstBrowseButton = browseButtons[0]
		if (!firstBrowseButton) {
			throw new Error('missing browse files button')
		}
		await user.click(firstBrowseButton)

		await waitFor(() => expect(screen.getByRole('button', { name: /new folder/i })).toBeInTheDocument())
	})

	it('keeps namespace display static for ordinary users', async () => {
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: false,
				can_manage_storage_classes: false,
				file_management_enabled: true,
				user_namespace: 'ns-admin',
			}),
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('ns-admin')).toBeInTheDocument()
		expect(screen.queryByRole('combobox', { name: /system namespace/i })).not.toBeInTheDocument()
		expect(screen.getByText('Namespace:')).toBeInTheDocument()
	})

	it('ignores stale admin namespaces for ordinary users and avoids admin resources', async () => {
		const user = userEvent.setup()
		viewerUIStore.actions.setNamespace('kube-system')
		const adminListNamespaces = vi.fn().mockResolvedValue([])
		const adminListStorageClasses = vi.fn().mockResolvedValue([])
		const adminCapabilities = vi.fn().mockResolvedValue({
			can_manage_pvcs: false,
			can_manage_storage_classes: false,
			file_management_enabled: true,
			user_namespace: 'ns-admin',
		})
		const getContext = vi.fn().mockResolvedValue({
			context_name: 'dev',
			namespace: 'ns-admin',
		})
		const listPVCs = vi.fn().mockResolvedValue([
			pvcFixture({ name: 'data', namespace: 'ns-admin', uid: 'uid-data' }),
		])
		const listStorageClasses = vi.fn().mockResolvedValue([
			storageClassFixture({ name: 'standard' }),
		])
		const api = createFakeViewerAPI({
			adminCapabilities,
			adminListNamespaces,
			adminListStorageClasses,
			getContext,
			listPVCs,
			listStorageClasses,
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('data')).toBeInTheDocument()
		expect(screen.getAllByText('ns-admin').length).toBeGreaterThan(0)
		expect(listPVCs).toHaveBeenCalledWith({ namespace: 'ns-admin' })
		expect(listPVCs).not.toHaveBeenCalledWith({ namespace: 'kube-system' })
		expect(adminListNamespaces).not.toHaveBeenCalled()
		expect(adminListStorageClasses).not.toHaveBeenCalled()

		await user.click(screen.getByRole('button', { name: /^refresh$/i }))

		await waitFor(() => expect(listPVCs).toHaveBeenCalledTimes(2))
		expect(getContext).toHaveBeenCalledTimes(2)
		expect(adminCapabilities).toHaveBeenCalledTimes(2)
		expect(listStorageClasses).toHaveBeenCalledTimes(2)
		expect(adminListNamespaces).not.toHaveBeenCalled()
		expect(adminListStorageClasses).not.toHaveBeenCalled()
	})

	it('uses the capability user namespace when admin context is outside the user namespace', async () => {
		const adminListNamespaces = vi.fn().mockResolvedValue([])
		const adminListStorageClasses = vi.fn().mockResolvedValue([])
		const listPVCs = vi.fn().mockResolvedValue([
			pvcFixture({ name: 'data', namespace: 'ns-admin', uid: 'uid-data' }),
		])
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: false,
				can_manage_storage_classes: false,
				file_management_enabled: true,
				user_namespace: 'ns-admin',
			}),
			adminListNamespaces,
			adminListStorageClasses,
			getContext: vi.fn().mockResolvedValue({
				context_name: 'system',
				namespace: 'kube-system',
			}),
			listPVCs,
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('data')).toBeInTheDocument()
		expect(screen.getAllByText('ns-admin').length).toBeGreaterThan(0)
		expect(screen.queryByText('kube-system')).not.toBeInTheDocument()
		expect(listPVCs).toHaveBeenCalledWith({ namespace: 'ns-admin' })
		expect(adminListNamespaces).not.toHaveBeenCalled()
		expect(adminListStorageClasses).not.toHaveBeenCalled()
	})

	it('prevents creating PVCs larger than available storage quota', async () => {
		const user = userEvent.setup()
		const createPVC = vi.fn()
		const api = createFakeViewerAPI({
			createPVC,
			getStorageQuota: vi.fn().mockResolvedValue(storageQuotaFixture({
				available_bytes: 5 * 1024 * 1024 * 1024,
				available_quantity: '5Gi',
			})),
			listPVCs: vi.fn().mockResolvedValue([]),
			listStorageClasses: vi.fn().mockResolvedValue([
				storageClassFixture({ name: 'standard' }),
			]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: /^create pvc$/i }))
		await user.type(screen.getByLabelText(/^name$/i), 'too-large')
		await user.clear(screen.getByLabelText(/^capacity$/i))
		await user.type(screen.getByLabelText(/^capacity$/i), '6')

		expect(await screen.findByText('Storage quota available: 5Gi.')).toBeInTheDocument()
		expect(screen.getByRole('button', { name: /^create$/i })).toBeDisabled()
		expect(createPVC).not.toHaveBeenCalled()
	})

	it('uses the current joined namespace when admin capabilities are disabled there', async () => {
		const adminListNamespaces = vi.fn().mockResolvedValue([])
		const adminListStorageClasses = vi.fn().mockResolvedValue([])
		const listPVCs = vi.fn().mockResolvedValue([
			pvcFixture({ name: 'joined-data', namespace: 'ns-joined', uid: 'uid-joined' }),
		])
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: false,
				can_manage_storage_classes: false,
				file_management_enabled: true,
				user_namespace: 'ns-admin',
			}),
			adminListNamespaces,
			adminListStorageClasses,
			getContext: vi.fn().mockResolvedValue({
				context_name: 'joined',
				namespace: 'ns-joined',
			}),
			listPVCs,
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('joined-data')).toBeInTheDocument()
		expect(screen.getAllByText('ns-joined').length).toBeGreaterThan(0)
		expect(listPVCs).toHaveBeenCalledWith({ namespace: 'ns-joined' })
		expect(listPVCs).not.toHaveBeenCalledWith({ namespace: 'ns-admin' })
		expect(adminListNamespaces).not.toHaveBeenCalled()
		expect(adminListStorageClasses).not.toHaveBeenCalled()
	})

	it('removes the in-app language switch and refreshes all storage queries from one action', async () => {
		const user = userEvent.setup()
		const adminCapabilities = vi.fn().mockResolvedValue({
			can_manage_pvcs: true,
			can_manage_storage_classes: true,
			file_management_enabled: true,
			user_namespace: 'ns-admin',
		})
		const adminListNamespaces = vi.fn().mockResolvedValue([
			{ is_current_context: true, name: 'ns-admin' },
			{ is_current_context: false, name: 'kube-system' },
		])
		const adminListStorageClasses = vi.fn().mockResolvedValue([
			storageClassFixture({ name: 'standard' }),
		])
		const listPVCs = vi.fn().mockResolvedValue([
			pvcFixture({ name: 'data', namespace: 'ns-admin', uid: 'uid-data' }),
		])
		const listStorageClasses = vi.fn().mockResolvedValue([
			storageClassFixture({ name: 'standard' }),
		])
		const getContext = vi.fn().mockResolvedValue({
			context_name: 'dev',
			namespace: 'ns-admin',
		})
		const api = createFakeViewerAPI({
			adminCapabilities,
			adminListNamespaces,
			adminListStorageClasses,
			getContext,
			listPVCs,
			listStorageClasses,
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('data')).toBeInTheDocument()
		expect(screen.queryByRole('button', { name: 'Locale' })).not.toBeInTheDocument()

		await user.click(screen.getByRole('button', { name: /^refresh$/i }))

		await waitFor(() => expect(listPVCs).toHaveBeenCalledTimes(2))
		expect(getContext).toHaveBeenCalledTimes(2)
		expect(adminCapabilities).toHaveBeenCalledTimes(2)
		expect(adminListNamespaces).toHaveBeenCalledTimes(2)
		expect(listStorageClasses).toHaveBeenCalledTimes(2)
		expect(adminListStorageClasses).toHaveBeenCalledTimes(2)
	})

	it('lets admins switch to system namespaces through existing PVC and session APIs', async () => {
		const user = userEvent.setup()
		const listPVCs = vi.fn()
			.mockResolvedValueOnce([])
			.mockResolvedValueOnce([
				pvcFixture({ name: 'system-data', namespace: 'kube-system', uid: 'system-uid' }),
			])
		const createViewerSession = vi.fn().mockResolvedValue(viewerSessionFixture({
			id: 'vs_system',
			namespace: 'kube-system',
			pvc_name: 'system-data',
			status: 'ready',
			token_ready: true,
		}))
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: true,
				can_manage_storage_classes: false,
				file_management_enabled: true,
				user_namespace: 'ns-admin',
			}),
			adminListNamespaces: vi.fn().mockResolvedValue([
				{ is_current_context: true, name: 'ns-admin' },
				{ is_current_context: false, name: 'kube-system' },
			]),
			createViewerSession,
			issueViewerToken: vi.fn().mockResolvedValue(viewerTokenFixture({
				viewer_session_id: 'vs_system',
				viewer_url: 'https://viewer.example.test',
			})),
			listPVCs,
		})

		renderWithProviders(<StorageAppShell api={api} />)

		const namespaceCombobox = await screen.findByRole('combobox', { name: /system namespace/i })
		await user.click(namespaceCombobox)
		await user.type(namespaceCombobox, 'kube')
		expect(screen.queryByRole('button', { name: /ns-admin/i })).not.toBeInTheDocument()
		await user.click(await screen.findByText('kube-system'))

		await waitFor(() => expect(listPVCs).toHaveBeenCalledWith({ namespace: 'kube-system' }))
		expect(await screen.findByText('system-data')).toBeInTheDocument()

		await user.click(screen.getByRole('button', { name: /browse files/i }))

		await waitFor(() => expect(createViewerSession).toHaveBeenCalledWith({
			namespace: 'kube-system',
			pvcName: 'system-data',
		}))
	})

	it('lets admins view all namespaces through the backend all token', async () => {
		const user = userEvent.setup()
		const listPVCs = vi.fn().mockImplementation(async ({ namespace }: { namespace: string }) => {
			if (namespace === ALL_NAMESPACES) {
				return [
					pvcFixture({ name: 'user-data', namespace: 'ns-admin', uid: 'uid-user' }),
					pvcFixture({ name: 'system-data', namespace: 'kube-system', uid: 'uid-system' }),
				]
			}
			return []
		})
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: true,
				can_manage_storage_classes: false,
				file_management_enabled: true,
				user_namespace: 'ns-admin',
			}),
			adminListNamespaces: vi.fn().mockResolvedValue([
				{ is_current_context: true, name: 'ns-admin' },
				{ is_current_context: false, name: 'kube-system' },
			]),
			listPVCs,
		})

		renderWithProviders(<StorageAppShell api={api} />)

		const namespaceCombobox = await screen.findByRole('combobox', { name: /system namespace/i })
		await user.type(namespaceCombobox, 'ns-admin')
		await user.click(namespaceCombobox)
		await user.click(await screen.findByText('All spaces'))

		await waitFor(() => expect(listPVCs).toHaveBeenCalledWith({ namespace: ALL_NAMESPACES }))
		expect(namespaceCombobox).toHaveValue('All spaces')
		expect(await screen.findByText('user-data')).toBeInTheDocument()
		expect(screen.getByRole('columnheader', { name: 'Namespace' })).toBeInTheDocument()
		expect(screen.getByRole('row', { name: /user-data.*ns-admin/i })).toBeInTheDocument()
		expect(screen.getByRole('row', { name: /system-data.*kube-system/i })).toBeInTheDocument()
		expect(screen.getByText('system-data')).toBeInTheDocument()
	})

	it('shows unknown mount state when mount detection is disabled', async () => {
		const api = createFakeViewerAPI({
			listPVCs: vi.fn().mockResolvedValue([
				pvcFixture({
					mount_status: 'unknown',
					name: 'archive',
					uid: 'archive-uid',
				}),
			]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('archive')).toBeInTheDocument()
		expect(screen.getAllByText('Mount detection off')).toHaveLength(3)
	})

	it('requires a concrete namespace when creating PVCs from the all namespaces view', async () => {
		const user = userEvent.setup()
		const createPVC = vi.fn().mockResolvedValue(pvcFixture({
			name: 'system-cache',
			namespace: 'kube-system',
			uid: 'system-cache-uid',
		}))
		const getStorageQuota = vi.fn().mockResolvedValue(storageQuotaFixture())
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: true,
				can_manage_storage_classes: false,
				file_management_enabled: true,
				user_namespace: 'ns-admin',
			}),
			adminListNamespaces: vi.fn().mockResolvedValue([
				{ is_current_context: true, name: 'ns-admin' },
				{ is_current_context: false, name: 'kube-system' },
			]),
			createPVC,
			getStorageQuota,
			listPVCs: vi.fn().mockResolvedValue([]),
			listStorageClasses: vi.fn().mockResolvedValue([
				storageClassFixture({ name: 'standard' }),
			]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		const namespaceCombobox = await screen.findByRole('combobox', { name: /system namespace/i })
		await user.click(namespaceCombobox)
		await user.click(await screen.findByText('All spaces'))
		await user.click(await screen.findByRole('button', { name: /^create pvc$/i }))
		await user.click(screen.getByRole('combobox', { name: /^target namespace$/i }))
		await user.click(await screen.findByRole('option', { name: /kube-system/i }))
		await user.type(screen.getByLabelText(/^name$/i), 'system-cache')
		await user.clear(screen.getByLabelText(/^capacity$/i))
		await user.type(screen.getByLabelText(/^capacity$/i), '5')
		await user.click(screen.getByRole('button', { name: /^create$/i }))

		await waitFor(() => expect(getStorageQuota).toHaveBeenCalledWith({ namespace: 'kube-system' }))
		expect(createPVC).toHaveBeenCalledWith(expect.objectContaining({
			name: 'system-cache',
			namespace: 'kube-system',
		}))
	})

	it('shows admin controls for dev kubeconfig when dev admin mode is enabled', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_DEV_ENABLE_ADMIN_MODE', 'true')
		sealosAuthorizationMockState.authorization = {
			accountAuthorizationHeader: '',
			authorizationHeader: 'Bearer dev',
			session: null,
			source: 'dev-kubeconfig',
		}
		const adminListNamespaces = vi.fn().mockResolvedValue([
			{ is_current_context: true, name: 'ns-admin' },
			{ is_current_context: false, name: 'kube-system' },
		])
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: true,
				can_manage_storage_classes: true,
				file_management_enabled: true,
				user_namespace: 'ns-admin',
			}),
			adminListNamespaces,
			getContext: vi.fn().mockResolvedValue({
				context_name: 'dev',
				namespace: 'ns-admin',
			}),
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByRole('combobox', { name: /system namespace/i })).toBeInTheDocument()
		expect(screen.getByRole('button', { name: 'StorageClasses' })).toBeInTheDocument()
		expect(adminListNamespaces).toHaveBeenCalled()
	})

	it('hides admin features when backend context is outside the Sealos user namespace', async () => {
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: false,
				can_manage_storage_classes: false,
				file_management_enabled: true,
				user_namespace: 'ns-admin',
			}),
			getContext: vi.fn().mockResolvedValue({
				context_name: 'dev',
				namespace: 'kube-system',
			}),
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('ns-admin')).toBeInTheDocument()
		expect(screen.queryByRole('combobox', { name: /system namespace/i })).not.toBeInTheDocument()
		expect(screen.queryByRole('button', { name: 'StorageClasses' })).not.toBeInTheDocument()
	})

	it('hides file management when the backend feature flag is disabled', async () => {
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: false,
				can_manage_storage_classes: false,
				file_management_enabled: false,
				user_namespace: 'ns-admin',
			}),
			listPVCs: vi.fn().mockResolvedValue([
				pvcFixture({ name: 'data', namespace: 'ns-admin', uid: 'uid-data' }),
			]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('data')).toBeInTheDocument()
		expect(screen.queryByRole('button', { name: /browse files/i })).not.toBeInTheDocument()
		expect(screen.queryByRole('button', { name: 'File management' })).not.toBeInTheDocument()
		expect(screen.queryByRole('button', { name: 'Recycle bin' })).not.toBeInTheDocument()
	})

	it('creates PVCs through the real dialog and optimistic mutation path', async () => {
		const user = userEvent.setup()
		const createPVC = vi.fn().mockResolvedValue(pvcFixture({
			name: 'cache-data',
			namespace: 'ns-admin',
			uid: 'cache-uid',
			capacity: '5Gi',
			capacity_bytes: 5 * 1024 * 1024 * 1024,
		}))
		const api = createFakeViewerAPI({
			createPVC,
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: /create pvc/i }))
		await user.type(screen.getByLabelText('Name'), 'cache-data')
		const capacityInput = screen.getByLabelText('Capacity')
		expect(screen.getByText('Gi')).toBeInTheDocument()
		await user.clear(capacityInput)
		expect(capacityInput).toHaveValue('')
		expect(screen.getByRole('button', { name: /^create$/i })).toBeDisabled()
		await user.type(capacityInput, '5')
		await user.click(screen.getByRole('button', { name: /^create$/i }))

		await waitFor(() => expect(createPVC).toHaveBeenCalledWith(expect.objectContaining({
			name: 'cache-data',
			namespace: 'ns-admin',
			capacity: '5Gi',
			capacityBytes: 5 * 1024 * 1024 * 1024,
			accessModes: ['ReadWriteOnce'],
			storageClassName: 'standard',
		})))
	})

	it('expands PVCs through a target capacity input', async () => {
		const user = userEvent.setup()
		const expandPVC = vi.fn().mockResolvedValue(pvcFixture({
			name: 'data',
			namespace: 'ns-admin',
			capacity: '20Gi',
			capacity_bytes: 20 * 1024 * 1024 * 1024,
		}))
		const api = createFakeViewerAPI({
			expandPVC,
			listPVCs: vi.fn().mockResolvedValue([
				pvcFixture({
					name: 'data',
					namespace: 'ns-admin',
					uid: 'uid-data',
					capacity: '10Gi',
					capacity_bytes: 10 * 1024 * 1024 * 1024,
				}),
			]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: /more actions/i }))
		await user.click(await screen.findByRole('menuitem', { name: /expand/i }))
		const capacityInput = await screen.findByLabelText(/target capacity/i)
		expect(screen.getAllByText('Gi').length).toBeGreaterThan(0)
		await user.clear(capacityInput)
		expect(capacityInput).toHaveValue('')
		expect(screen.getByRole('button', { name: /^expand pvc$/i })).toBeDisabled()
		await user.clear(capacityInput)
		await user.type(capacityInput, '10')
		expect(screen.getByRole('button', { name: /^expand pvc$/i })).toBeDisabled()
		await user.clear(capacityInput)
		await user.type(capacityInput, '20')
		await user.click(screen.getByRole('button', { name: /^expand pvc$/i }))

		await waitFor(() => expect(expandPVC).toHaveBeenCalledWith({
			namespace: 'ns-admin',
			name: 'data',
			capacity: '20Gi',
			capacityBytes: 20 * 1024 * 1024 * 1024,
		}))
	})

	it('shows backend details in PVC mutation error toasts', async () => {
		const user = userEvent.setup()
		const detail = 'persistentvolumeclaims "cache-data" is forbidden: exceeded quota: quota-ns-admin'
		const createPVC = vi.fn().mockRejectedValue(new ViewerApiError({
			code: 'PVC_CREATE_FORBIDDEN',
			details: {
				message: detail,
			},
			message: detail,
			status: 403,
		}))
		const api = createFakeViewerAPI({
			createPVC,
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: /create pvc/i }))
		await user.type(screen.getByLabelText('Name'), 'cache-data')
		const capacityInput = screen.getByLabelText('Capacity')
		await user.clear(capacityInput)
		await user.type(capacityInput, '5')
		await user.click(screen.getByRole('button', { name: /^create$/i }))

		expect(await screen.findByText('You do not have permission to create PVCs in this namespace.')).toBeInTheDocument()
		expect(await screen.findByText(detail)).toBeInTheDocument()
	})

	it('creates PVCs with any StorageClass regardless of legacy policy metadata', async () => {
		const user = userEvent.setup()
		const createPVC = vi.fn().mockResolvedValue(pvcFixture({ name: 'hidden-data' }))
		const api = createFakeViewerAPI({
			createPVC,
			listPVCs: vi.fn().mockResolvedValue([]),
			listStorageClasses: vi.fn().mockResolvedValue([
				storageClassFixture({
					name: 'standard',
				}),
				storageClassFixture({
					name: 'shared',
					is_default: false,
				}),
				storageClassFixture({
					name: 'hidden',
					is_default: false,
				}),
			]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: /create pvc/i }))
		await user.type(screen.getByLabelText('Name'), 'hidden-data')
		await user.click(screen.getByRole('combobox', { name: /storage class/i }))
		await user.click(await screen.findByRole('option', { name: 'hidden' }))
		await user.click(screen.getByRole('combobox', { name: /access modes/i }))
		await user.click(await screen.findByRole('option', { name: 'ReadWriteMany' }))
		await user.click(screen.getByRole('button', { name: /^create$/i }))

		await waitFor(() => expect(createPVC).toHaveBeenCalledWith(expect.objectContaining({
			accessModes: ['ReadWriteMany'],
			storageClassName: 'hidden',
		})))
	})

	it('disables PVC creation when no StorageClass exists', async () => {
		const user = userEvent.setup()
		const createPVC = vi.fn()
		const api = createFakeViewerAPI({
			createPVC,
			listPVCs: vi.fn().mockResolvedValue([]),
			listStorageClasses: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: /create pvc/i }))
		await user.type(screen.getByLabelText('Name'), 'cache-data')

		expect(screen.getByText(/no storageclass exists/i)).toBeInTheDocument()
		expect(screen.getByRole('button', { name: /^create$/i })).toBeDisabled()
		expect(createPVC).not.toHaveBeenCalled()
	})

	it('hides PVC creation when the backend feature gate is disabled', async () => {
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: false,
				can_manage_storage_classes: false,
				file_management_enabled: true,
				pvc_creation_enabled: false,
				user_namespace: 'ns-admin',
			}),
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await waitFor(() => expect(screen.queryByRole('button', { name: /create pvc/i })).not.toBeInTheDocument())
	})

	it('orders the create PVC form as storage class before access mode', async () => {
		const user = userEvent.setup()
		const api = createFakeViewerAPI({
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: /create pvc/i }))

		const dialog = screen.getByRole('dialog')
		const storageClass = within(dialog).getByText('Storage class')
		const accessModes = within(dialog).getByText('Access modes')

		expect(storageClass.compareDocumentPosition(accessModes) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
	})

	it('shows localized API errors', async () => {
		const api = createFakeViewerAPI({
			listPVCs: vi.fn().mockRejectedValue(new ViewerApiError({
				code: 'PVC_ACCESS_DENIED',
				message: 'denied',
				status: 403,
			})),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText(/permission to access/i)).toBeInTheDocument()
	})

	it('shows admin StorageClass management only for capable users and opens describe output', async () => {
		const user = userEvent.setup()
		const adminDescribeStorageClass = vi.fn().mockResolvedValue(storageClassDescribeFixture({
			describe: 'Name: standard\nProvisioner: kubernetes.io/no-provisioner',
		}))
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: false,
				can_manage_storage_classes: true,
				file_management_enabled: true,
				user_namespace: 'ns-admin',
			}),
			adminDescribeStorageClass,
			adminListStorageClasses: vi.fn().mockResolvedValue([
				storageClassFixture({ name: 'standard' }),
			]),
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: 'StorageClasses' }))
		expect(await screen.findByRole('heading', { name: 'StorageClasses' })).toBeInTheDocument()
		expect(screen.getByText('standard')).toBeInTheDocument()

		await user.click(screen.getByRole('button', { name: 'Describe' }))

		await waitFor(() => expect(adminDescribeStorageClass).toHaveBeenCalledWith('standard'))
		expect(await screen.findByDisplayValue(/Name: standard/)).toBeInTheDocument()
	})

	it('creates, edits, and deletes StorageClasses through the admin dialogs', async () => {
		const user = userEvent.setup()
		const adminCreateStorageClass = vi.fn().mockResolvedValue(storageClassFixture({ name: 'created' }))
		const adminDeleteStorageClass = vi.fn().mockResolvedValue(storageClassFixture({ name: 'standard' }))
		const adminGetStorageClassYAML = vi.fn().mockResolvedValue(storageClassYAMLFixture({
			name: 'standard',
			yaml: 'apiVersion: storage.k8s.io/v1\nkind: StorageClass\nmetadata:\n  name: standard\nprovisioner: test.io/standard\n',
		}))
		const adminUpdateStorageClass = vi.fn().mockResolvedValue(storageClassFixture({ name: 'standard' }))
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: false,
				can_manage_storage_classes: true,
				file_management_enabled: true,
				user_namespace: 'ns-admin',
			}),
			adminCreateStorageClass,
			adminDeleteStorageClass,
			adminGetStorageClassYAML,
			adminListStorageClasses: vi.fn().mockResolvedValue([
				storageClassFixture({
					name: 'standard',
					allow_volume_expansion: true,
					reclaim_policy: 'Retain',
					volume_binding_mode: 'WaitForFirstConsumer',
				}),
			]),
			adminUpdateStorageClass,
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: 'StorageClasses' }))
		expect(await screen.findByRole('columnheader', { name: 'Reclaim policy' })).toBeInTheDocument()
		expect(screen.getByRole('columnheader', { name: 'Volume binding mode' })).toBeInTheDocument()
		expect(screen.getByRole('columnheader', { name: 'Allow volume expansion' })).toBeInTheDocument()
		expect(screen.getByRole('columnheader', { name: 'PVC usage' })).toBeInTheDocument()
		expect(screen.queryByRole('columnheader', { name: 'Create PVC visibility' })).not.toBeInTheDocument()
		expect(screen.queryByRole('columnheader', { name: 'Access modes' })).not.toBeInTheDocument()
		expect(screen.queryAllByRole('button', { name: /^refresh$/i })).toHaveLength(1)
		expect(await screen.findByText('Retain')).toBeInTheDocument()
		expect(screen.getByText('WaitForFirstConsumer')).toBeInTheDocument()
		expect(screen.getByText('Yes')).toBeInTheDocument()
		await user.click(await screen.findByRole('button', { name: 'Create StorageClass' }))
		const createEditor = await screen.findByLabelText('Monaco editor')
		await user.clear(createEditor)
		await user.type(createEditor, 'apiVersion: storage.k8s.io/v1\nkind: StorageClass\nmetadata:\n  name: created\nprovisioner: test.io/created\n')
		await user.click(screen.getByRole('button', { name: 'Save' }))
		await waitFor(() => expect(adminCreateStorageClass).toHaveBeenCalledWith(expect.objectContaining({
			yaml: expect.stringContaining('name: created'),
		})))

		await user.click(await screen.findByRole('button', { name: 'Edit' }))
		const editEditor = await screen.findByLabelText('Monaco editor')
		expect(screen.getByRole('dialog')).toHaveClass('h-[88vh]')
		await waitFor(() => expect((editEditor as HTMLTextAreaElement).value).toContain('name: standard'))
		await user.clear(editEditor)
		await user.type(editEditor, 'apiVersion: storage.k8s.io/v1\nkind: StorageClass\nmetadata:\n  name: standard\nprovisioner: test.io/updated\n')
		await user.click(screen.getByRole('button', { name: 'Save' }))
		await waitFor(() => expect(adminUpdateStorageClass).toHaveBeenCalledWith('standard', expect.objectContaining({
			yaml: expect.stringContaining('test.io/updated'),
		})))

		expect(screen.queryByRole('button', { name: 'Policy' })).not.toBeInTheDocument()

		await user.click(await screen.findByRole('button', { name: 'Delete' }))
		await user.type(screen.getByLabelText('Type PVC name to confirm'), 'standard')
		await user.click(screen.getByRole('button', { name: 'Delete' }))
		await waitFor(() => expect(adminDeleteStorageClass).toHaveBeenCalledWith('standard'))
	})

	it('disables StorageClass deletion for external or in-use classes', async () => {
		const user = userEvent.setup()
		const adminDeleteStorageClass = vi.fn()
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: false,
				can_manage_storage_classes: true,
				file_management_enabled: true,
				user_namespace: 'ns-admin',
			}),
			adminDeleteStorageClass,
			adminListStorageClasses: vi.fn().mockResolvedValue([
				storageClassFixture({
					name: 'external',
					delete_blocked_reason: 'not_managed',
					managed_by_storage_manager: false,
				}),
				storageClassFixture({
					name: 'in-use',
					delete_blocked_reason: 'in_use',
					in_use_pvc_count: 2,
				}),
				storageClassFixture({
					name: 'managed',
				}),
			]),
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: 'StorageClasses' }))
		const externalRow = screen.getByText('external').closest('tr')
		const inUseRow = screen.getByText('in-use').closest('tr')
		const managedRow = screen.getByText('managed').closest('tr')
		if (!externalRow || !inUseRow || !managedRow) {
			throw new Error('missing StorageClass row')
		}
		expect(within(externalRow).getByRole('button', { name: 'Delete' })).toBeDisabled()
		expect(within(inUseRow).getByRole('button', { name: 'Delete' })).toBeDisabled()
		expect(within(inUseRow).getByText('2')).toBeInTheDocument()
		await user.click(within(managedRow).getByRole('button', { name: 'Delete' }))
		await user.type(screen.getByLabelText('Type PVC name to confirm'), 'managed')
		await user.click(screen.getByRole('button', { name: 'Delete' }))

		await waitFor(() => expect(adminDeleteStorageClass).toHaveBeenCalledWith('managed'))
	})

	it('shows PVC usage states from backend volume metrics in the second column', async () => {
		const user = userEvent.setup()
		const api = createFakeViewerAPI({
			listPVCs: vi.fn().mockResolvedValue([
				pvcFixture({
					name: 'ready-data',
					uid: 'uid-ready-data',
					capacity: '10Gi',
					capacity_bytes: 10 * 1024 * 1024 * 1024,
					volume_stats: {
						available_bytes: 7 * 1024 * 1024 * 1024,
						metric_capacity_bytes: 10 * 1024 * 1024 * 1024,
						sample_time: '2026-06-10T09:46:12Z',
						source: 'victoria-metrics',
						status: 'ready',
						used_bytes: 3 * 1024 * 1024 * 1024,
					},
				}),
				pvcFixture({
					name: 'missing-data',
					uid: 'uid-missing-data',
					volume_stats: undefined,
				}),
				pvcFixture({
					name: 'mismatch-data',
					uid: 'uid-mismatch-data',
					capacity: '1Gi',
					capacity_bytes: 1024 * 1024 * 1024,
					mounted: true,
					mounted_pods: [{
						name: 're-irldxgfr-minio-0',
						namespace: 'ns-admin',
						node_name: 'sealos-gpu-node0',
						phase: 'Running',
						read_only: false,
					}],
					volume_stats: {
						available_bytes: 90 * 1024 * 1024 * 1024,
						metric_capacity_bytes: 418 * 1024 * 1024 * 1024,
						sample_time: undefined,
						source: 'victoria-metrics',
						status: 'mismatched',
						used_bytes: 307 * 1024 * 1024 * 1024,
					},
				}),
			]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByRole('columnheader', { name: 'Usage' })).toBeInTheDocument()
		expect(await screen.findByText('30%')).toBeInTheDocument()
		expect(screen.getByText('3 GiB / 10 GiB')).toBeInTheDocument()
		expect(screen.queryByText('Free 7 GiB')).not.toBeInTheDocument()
		expect(screen.getByRole('progressbar', { name: 'ready-data PVC usage' }).querySelector('[data-slot="progress-indicator"]')).toHaveStyle({
			transform: 'translateX(-70%)',
		})
		expect(screen.getByText('Not collected')).toBeInTheDocument()
		expect(screen.getByText('307 GiB / 418 GiB')).toBeInTheDocument()
		expect(screen.getByText('73%')).toBeInTheDocument()
		expect(screen.getByRole('progressbar', { name: 'mismatch-data PVC usage' }).querySelector('[data-slot="progress-indicator"]')).toHaveStyle({
			transform: 'translateX(-27%)',
		})
		expect(screen.queryByText('307 GiB / 1 GiB')).not.toBeInTheDocument()
		expect(screen.queryByText('Free 90 GiB')).not.toBeInTheDocument()
		expect(screen.queryByText('Metrics mismatch')).not.toBeInTheDocument()
		expect(screen.queryByText('Reported FS 418 GiB')).not.toBeInTheDocument()
		expect(screen.queryByText('default')).not.toBeInTheDocument()

		const mismatchButton = screen.getByRole('button', { name: 'mismatch-data metrics mismatch' })
		await user.hover(mismatchButton)
		expect(await screen.findAllByText('Metrics mismatch')).not.toHaveLength(0)
		expect(screen.getAllByText('PVC request 1 GiB')).not.toHaveLength(0)
		expect(screen.queryByText(/re-irldxgfr-minio-0 · ns-admin/)).not.toBeInTheDocument()
		await user.unhover(mismatchButton)
		const readyProgress = screen.getByRole('progressbar', { name: 'ready-data PVC usage' })
		expect(readyProgress).toHaveAttribute('title', 'Free 7 GiB')
		const mismatchRow = screen.getByText('mismatch-data').closest('tr')
		if (!mismatchRow) {
			throw new Error('missing mismatch-data row')
		}
		expect(mismatchRow).toHaveClass('h-16')
		const mountedButton = within(mismatchRow).getByRole('button', { name: 'Mounted' })
		expect(mountedButton).toHaveAttribute('title', 're-irldxgfr-minio-0')
		await user.hover(mountedButton)
	})

	it('stops restarting the viewer flow after token recovery is exhausted', async () => {
		const user = userEvent.setup()
		const createViewerSession = vi
			.fn()
			.mockResolvedValueOnce(viewerSessionFixture({
				id: 'vs_old',
				pod_session_id: 'ps_old',
				status: 'ready',
				token_ready: true,
			}))
			.mockResolvedValueOnce(viewerSessionFixture({
				id: 'vs_new',
				pod_session_id: 'ps_new',
				status: 'ready',
				token_ready: true,
			}))
		const issueViewerToken = vi.fn().mockRejectedValue(new ViewerApiError({
			code: 'POD_SESSION_NOT_FOUND',
			message: 'Pod session no longer exists',
			status: 404,
		}))
		const api = createFakeViewerAPI({
			createViewerSession,
			issueViewerToken,
			listPVCs: vi.fn().mockResolvedValue([
				pvcFixture({ name: 'data', namespace: 'ns-admin', uid: 'uid-data' }),
			]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: /browse files/i }))

		await waitFor(() => expect(createViewerSession).toHaveBeenCalledTimes(2), { timeout: 3_000 })
		await waitFor(() => expect(issueViewerToken).toHaveBeenCalledTimes(2), { timeout: 3_000 })
		await new Promise(resolve => window.setTimeout(resolve, 100))
		expect(createViewerSession).toHaveBeenCalledTimes(2)
	})
})
