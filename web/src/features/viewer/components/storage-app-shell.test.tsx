import type { SessionV1 } from '@labring/sealos-desktop-sdk'
import type { SealosAuthorizationState } from '@/services/sealos/sealos-authorization'

import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ViewerApiError } from '@/features/viewer/api/viewer-error'
import { StorageAppShell } from '@/features/viewer/components/storage-app-shell'
import { viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'
import {
	createFakeViewerAPI,
	pvcFixture,
	storageClassDescribeFixture,
	storageClassFixture,
	storageClassYAMLFixture,
	viewerSessionFixture,
	viewerTokenFixture,
} from '@/features/viewer/test/fakes'
import { renderWithProviders } from '@/test/render'

const sealosAuthorizationMockState = vi.hoisted(() => ({
	authorization: {
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
			}),
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('ns-admin')).toBeInTheDocument()
		expect(screen.queryByRole('combobox', { name: /system namespace/i })).not.toBeInTheDocument()
		expect(screen.getByText('Namespace:')).toBeInTheDocument()
	})

	it('removes the in-app language switch and refreshes all storage queries from one action', async () => {
		const user = userEvent.setup()
		const adminCapabilities = vi.fn().mockResolvedValue({
			can_manage_pvcs: true,
			can_manage_storage_classes: true,
			file_management_enabled: true,
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

	it('shows admin controls for dev kubeconfig when dev admin mode is enabled', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_DEV_ENABLE_ADMIN_MODE', 'true')
		sealosAuthorizationMockState.authorization = {
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
				can_manage_pvcs: true,
				can_manage_storage_classes: true,
				file_management_enabled: true,
			}),
			getContext: vi.fn().mockResolvedValue({
				context_name: 'dev',
				namespace: 'kube-system',
			}),
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('kube-system')).toBeInTheDocument()
		expect(screen.queryByRole('combobox', { name: /system namespace/i })).not.toBeInTheDocument()
		expect(screen.queryByRole('button', { name: 'StorageClasses' })).not.toBeInTheDocument()
	})

	it('hides file management when the backend feature flag is disabled', async () => {
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: false,
				can_manage_storage_classes: false,
				file_management_enabled: false,
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

	it('limits PVC access modes to the selected StorageClass policy', async () => {
		const user = userEvent.setup()
		const createPVC = vi.fn().mockResolvedValue(pvcFixture({ name: 'shared-data' }))
		const api = createFakeViewerAPI({
			createPVC,
			listPVCs: vi.fn().mockResolvedValue([]),
			listStorageClasses: vi.fn().mockResolvedValue([
				storageClassFixture({
					name: 'standard',
					allowed_access_modes: ['ReadWriteOnce'],
				}),
				storageClassFixture({
					name: 'shared',
					allowed_access_modes: ['ReadWriteMany'],
					is_default: false,
				}),
				storageClassFixture({
					name: 'hidden',
					allowed_access_modes: ['ReadWriteMany'],
					annotation_status: 'hidden',
					is_default: false,
					visible_in_create: false,
				}),
			]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: /create pvc/i }))
		await user.type(screen.getByLabelText('Name'), 'shared-data')
		await user.click(screen.getByRole('combobox', { name: /storage class/i }))
		await user.click(await screen.findByRole('option', { name: 'shared' }))
		expect(screen.queryByRole('option', { name: 'hidden' })).not.toBeInTheDocument()
		await waitFor(() => expect(screen.getByRole('combobox', { name: /access modes/i })).toHaveTextContent('ReadWriteMany'))
		await user.click(screen.getByRole('button', { name: /^create$/i }))

		await waitFor(() => expect(createPVC).toHaveBeenCalledWith(expect.objectContaining({
			accessModes: ['ReadWriteMany'],
			storageClassName: 'shared',
		})))
	})

	it('disables PVC creation when no StorageClass is visible for create', async () => {
		const user = userEvent.setup()
		const createPVC = vi.fn()
		const api = createFakeViewerAPI({
			createPVC,
			listPVCs: vi.fn().mockResolvedValue([]),
			listStorageClasses: vi.fn().mockResolvedValue([
				storageClassFixture({
					allowed_access_modes: [],
					annotation_status: 'invalid',
					visible_in_create: true,
				}),
				storageClassFixture({
					name: 'hidden',
					annotation_status: 'hidden',
					visible_in_create: false,
				}),
			]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: /create pvc/i }))
		await user.type(screen.getByLabelText('Name'), 'cache-data')

		expect(screen.getByText(/no storageclass is available/i)).toBeInTheDocument()
		expect(screen.getByRole('button', { name: /^create$/i })).toBeDisabled()
		expect(createPVC).not.toHaveBeenCalled()
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
		const adminUpdateStorageClassPolicy = vi.fn().mockResolvedValue(storageClassFixture({
			name: 'standard',
			allowed_access_modes: ['ReadWriteMany'],
		}))
		const adminUpdateStorageClass = vi.fn().mockResolvedValue(storageClassFixture({ name: 'standard' }))
		const api = createFakeViewerAPI({
			adminCapabilities: vi.fn().mockResolvedValue({
				can_manage_pvcs: false,
				can_manage_storage_classes: true,
				file_management_enabled: true,
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
			adminUpdateStorageClassPolicy,
			listPVCs: vi.fn().mockResolvedValue([]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		await user.click(await screen.findByRole('button', { name: 'StorageClasses' }))
		expect(await screen.findByRole('columnheader', { name: 'Reclaim policy' })).toBeInTheDocument()
		expect(screen.getByRole('columnheader', { name: 'Volume binding mode' })).toBeInTheDocument()
		expect(screen.getByRole('columnheader', { name: 'Allow volume expansion' })).toBeInTheDocument()
		expect(screen.getByRole('columnheader', { name: 'PVC usage' })).toBeInTheDocument()
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

		await user.click(await screen.findByRole('button', { name: 'Policy' }))
		await user.click(screen.getByRole('checkbox', { name: 'ReadWriteOnce' }))
		await user.click(screen.getByRole('checkbox', { name: 'ReadWriteMany' }))
		await user.click(screen.getByRole('button', { name: 'Save' }))
		await waitFor(() => expect(adminUpdateStorageClassPolicy).toHaveBeenCalledWith('standard', {
			allowedAccessModes: ['ReadWriteMany'],
			visibleInCreate: true,
		}))

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
