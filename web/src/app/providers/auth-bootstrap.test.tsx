import { render, screen, waitFor } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { AuthBootstrap } from '@/app/providers/auth-bootstrap'
import { initializeSealosAuthorization, resetSealosAuthorizationForTest } from '@/services/sealos/sealos-authorization'

vi.mock('@/services/sealos/sealos-authorization', () => ({
	initializeSealosAuthorization: vi.fn(),
	resetSealosAuthorizationForTest: vi.fn(),
}))

const initializeSealosAuthorizationMock = vi.mocked(initializeSealosAuthorization)

describe('auth bootstrap', () => {
	afterEach(() => {
		initializeSealosAuthorizationMock.mockReset()
		vi.mocked(resetSealosAuthorizationForTest).mockReset()
	})

	it('keeps children unmounted until Sealos authorization is ready', async () => {
		let resolveAuthorization: () => void = () => undefined
		initializeSealosAuthorizationMock.mockReturnValue(
			new Promise((resolve) => {
				resolveAuthorization = () => resolve({
					authorizationHeader: 'Bearer test',
					session: null,
					source: 'dev-kubeconfig',
				})
			}),
		)

		render(
			<AuthBootstrap>
				<div>Storage app mounted</div>
			</AuthBootstrap>,
		)

		expect(screen.getByText('Initializing Sealos authorization...')).toBeVisible()
		expect(screen.queryByText('Storage app mounted')).not.toBeInTheDocument()

		resolveAuthorization()

		await expect(screen.findByText('Storage app mounted')).resolves.toBeVisible()
	})

	it('shows an auth error and keeps children unmounted when bootstrap fails', async () => {
		initializeSealosAuthorizationMock.mockRejectedValue(new Error('SDK session unavailable'))

		render(
			<AuthBootstrap>
				<div>Storage app mounted</div>
			</AuthBootstrap>,
		)

		await expect(screen.findByText('Authorization unavailable')).resolves.toBeVisible()
		expect(screen.getByText('SDK session unavailable')).toBeVisible()
		expect(screen.queryByText('Storage app mounted')).not.toBeInTheDocument()
	})

	it('reloads the page from the retry action', async () => {
		const reload = vi.fn()
		const originalLocation = window.location
		initializeSealosAuthorizationMock.mockRejectedValue(new Error('SDK session unavailable'))
		Object.defineProperty(window, 'location', {
			configurable: true,
			value: { ...originalLocation, reload },
		})

		render(
			<AuthBootstrap>
				<div>Storage app mounted</div>
			</AuthBootstrap>,
		)

		await userEvent.click(await screen.findByRole('button', { name: 'Retry' }))

		expect(reload).toHaveBeenCalledTimes(1)

		Object.defineProperty(window, 'location', {
			configurable: true,
			value: originalLocation,
		})
	})

	it('does not update state after unmount', async () => {
		let resolveAuthorization: () => void = () => undefined
		initializeSealosAuthorizationMock.mockReturnValue(
			new Promise((resolve) => {
				resolveAuthorization = () => resolve({
					authorizationHeader: 'Bearer test',
					session: null,
					source: 'dev-kubeconfig',
				})
			}),
		)
		const { unmount } = render(
			<AuthBootstrap>
				<div>Storage app mounted</div>
			</AuthBootstrap>,
		)

		unmount()
		resolveAuthorization()

		await waitFor(() => {
			expect(screen.queryByText('Storage app mounted')).not.toBeInTheDocument()
		})
	})
})
