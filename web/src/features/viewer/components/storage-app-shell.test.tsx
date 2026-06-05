import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ViewerApiError } from '@/features/viewer/api/viewer-error'
import { StorageAppShell } from '@/features/viewer/components/storage-app-shell'
import { viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'
import { createFakeViewerAPI, pvcFixture, viewerSessionFixture, viewerTokenFixture } from '@/features/viewer/test/fakes'
import { renderWithProviders } from '@/test/render'

describe('storageAppShell', () => {
	beforeEach(() => {
		viewerUIStore.actions.reset()
	})

	it('renders PVCs, filters them, launches File Browser, and shows real file manager state', async () => {
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
		expect(screen.queryByRole('columnheader', { name: /capacity/i })).not.toBeInTheDocument()
		expect(screen.queryByText('10Gi')).not.toBeInTheDocument()
		expect(screen.getAllByText('ns-admin').length).toBeGreaterThan(0)
		expect(listPVCs).toHaveBeenCalledWith({ namespace: 'ns-admin' })
		expect(listPVCs).not.toHaveBeenCalledWith({ namespace: 'default' })

		await user.type(screen.getByLabelText('Search'), 'mysql')
		await waitFor(() => expect(screen.queryByText('logs')).not.toBeInTheDocument())
		expect(screen.getByText('mysql-data')).toBeInTheDocument()

		await user.click(screen.getByRole('button', { name: /browse files/i }))

		await waitFor(() => expect(screen.getByRole('button', { name: /new folder/i })).toBeInTheDocument())
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
		await user.clear(capacityInput)
		await user.type(capacityInput, '5')
		await user.click(screen.getByRole('button', { name: /^create$/i }))

		await waitFor(() => expect(createPVC).toHaveBeenCalledWith(expect.objectContaining({
			name: 'cache-data',
			namespace: 'ns-admin',
			capacity: '5Gi',
			capacityBytes: 5 * 1024 * 1024 * 1024,
		})))
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
