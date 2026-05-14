import { screen, waitFor } from '@testing-library/react'
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

	it('renders PVCs, filters them, launches a viewer, and shows token-backed open action', async () => {
		const user = userEvent.setup()
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
			listPVCs: vi.fn().mockResolvedValue([
				pvcFixture({ name: 'mysql-data', uid: 'uid-1' }),
				pvcFixture({ name: 'logs', uid: 'uid-2' }),
			]),
		})

		renderWithProviders(<StorageAppShell api={api} />)

		expect(await screen.findByText('mysql-data')).toBeInTheDocument()
		expect(screen.getByText('logs')).toBeInTheDocument()

		await user.type(screen.getByLabelText('Search'), 'mysql')
		expect(screen.getByText('mysql-data')).toBeInTheDocument()
		expect(screen.queryByText('logs')).not.toBeInTheDocument()

		await user.click(screen.getByRole('button', { name: /launch viewer/i }))
		await user.click(screen.getByRole('tab', { name: 'Viewer' }))

		await waitFor(() => expect(screen.getByText('https://viewer.example.test')).toBeInTheDocument())
		expect(screen.getByRole('link', { name: /open file browser/i })).toHaveAttribute('href', 'https://viewer.example.test')
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
})
