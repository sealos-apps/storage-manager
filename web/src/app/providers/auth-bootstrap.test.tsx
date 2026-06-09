import { render, screen, waitFor } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { AuthBootstrap } from '@/app/providers/auth-bootstrap'
import { i18n } from '@/i18n'
import { initializeSealosAuthorization, resetSealosAuthorizationForTest } from '@/services/sealos/sealos-authorization'

const sdkLanguageMockState = vi.hoisted(() => {
	const listeners = new Map<string, (data?: unknown) => unknown>()
	const getLanguage = vi.fn()
	const addAppEventListen = vi.fn((name: string, fn: (data?: unknown) => unknown) => {
		listeners.set(name, fn)
		return () => listeners.delete(name)
	})
	return {
		addAppEventListen,
		getLanguage,
		listeners,
	}
})

vi.mock('@/services/sealos/sealos-authorization', () => ({
	initializeSealosAuthorization: vi.fn(),
	resetSealosAuthorizationForTest: vi.fn(),
}))

vi.mock('@labring/sealos-desktop-sdk', () => ({
	EVENT_NAME: {
		CHANGE_I18N: 'change_i18n',
	},
}))

vi.mock('@labring/sealos-desktop-sdk/app', () => ({
	sealosApp: {
		addAppEventListen: sdkLanguageMockState.addAppEventListen,
		getLanguage: sdkLanguageMockState.getLanguage,
	},
}))

const initializeSealosAuthorizationMock = vi.mocked(initializeSealosAuthorization)

describe('auth bootstrap', () => {
	afterEach(() => {
		initializeSealosAuthorizationMock.mockReset()
		sdkLanguageMockState.getLanguage.mockReset()
		sdkLanguageMockState.addAppEventListen.mockClear()
		sdkLanguageMockState.listeners.clear()
		vi.mocked(resetSealosAuthorizationForTest).mockReset()
		delete window.__SEALOS_STORAGE_MANAGER_CONFIG__
		vi.unstubAllEnvs()
		void i18n.changeLanguage('zh')
		vi.resetModules()
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

	it('applies forced language before mounting children', async () => {
		window.__SEALOS_STORAGE_MANAGER_CONFIG__ = {
			forcedLanguage: 'en',
		}
		initializeSealosAuthorizationMock.mockResolvedValue({
			authorizationHeader: 'Bearer test',
			session: {
				user: {
					language: 'zh',
				},
			} as never,
			source: 'sdk',
		})

		render(
			<AuthBootstrap>
				<div>Storage app mounted</div>
			</AuthBootstrap>,
		)

		await expect(screen.findByText('Storage app mounted')).resolves.toBeVisible()
		expect(i18n.language).toBe('en')
	})

	it('uses the Desktop SDK language when forced language is empty', async () => {
		sdkLanguageMockState.getLanguage.mockResolvedValue({ lng: 'en' })
		initializeSealosAuthorizationMock.mockResolvedValue({
			authorizationHeader: 'Bearer test',
			session: {
				user: {
					language: 'zh',
				},
			} as never,
			source: 'sdk',
		})

		render(
			<AuthBootstrap>
				<div>Storage app mounted</div>
			</AuthBootstrap>,
		)

		await expect(screen.findByText('Storage app mounted')).resolves.toBeVisible()
		expect(i18n.language).toBe('en')
	})

	it('falls back to SDK session language when Desktop SDK language is unavailable', async () => {
		sdkLanguageMockState.getLanguage.mockRejectedValue(new Error('language unavailable'))
		initializeSealosAuthorizationMock.mockResolvedValue({
			authorizationHeader: 'Bearer test',
			session: {
				user: {
					language: 'en',
				},
			} as never,
			source: 'sdk',
		})

		render(
			<AuthBootstrap>
				<div>Storage app mounted</div>
			</AuthBootstrap>,
		)

		await expect(screen.findByText('Storage app mounted')).resolves.toBeVisible()
		expect(i18n.language).toBe('en')
	})

	it('updates language from Desktop language change events', async () => {
		sdkLanguageMockState.getLanguage.mockResolvedValue({ lng: 'en' })
		initializeSealosAuthorizationMock.mockResolvedValue({
			authorizationHeader: 'Bearer test',
			session: null,
			source: 'sdk',
		})

		render(
			<AuthBootstrap>
				<div>Storage app mounted</div>
			</AuthBootstrap>,
		)

		await expect(screen.findByText('Storage app mounted')).resolves.toBeVisible()
		expect(i18n.language).toBe('en')

		await sdkLanguageMockState.listeners.get('change_i18n')?.({ currentLanguage: 'zh' })

		await waitFor(() => expect(i18n.language).toBe('zh'))
	})

	it('keeps forced language when Desktop language change events arrive', async () => {
		window.__SEALOS_STORAGE_MANAGER_CONFIG__ = {
			forcedLanguage: 'en',
		}
		sdkLanguageMockState.getLanguage.mockResolvedValue({ lng: 'zh' })
		initializeSealosAuthorizationMock.mockResolvedValue({
			authorizationHeader: 'Bearer test',
			session: null,
			source: 'sdk',
		})

		render(
			<AuthBootstrap>
				<div>Storage app mounted</div>
			</AuthBootstrap>,
		)

		await expect(screen.findByText('Storage app mounted')).resolves.toBeVisible()
		await sdkLanguageMockState.listeners.get('change_i18n')?.({ currentLanguage: 'zh' })

		expect(sdkLanguageMockState.addAppEventListen).not.toHaveBeenCalled()
		expect(i18n.language).toBe('en')
	})

	it('skips Desktop language APIs when the dev SDK bypass env var is enabled', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_DEV_DISABLE_SEALOS_DESKTOP_SDK', 'true')
		initializeSealosAuthorizationMock.mockResolvedValue({
			authorizationHeader: 'Bearer test',
			session: null,
			source: 'dev-kubeconfig',
		})

		render(
			<AuthBootstrap>
				<div>Storage app mounted</div>
			</AuthBootstrap>,
		)

		await expect(screen.findByText('Storage app mounted')).resolves.toBeVisible()
		expect(sdkLanguageMockState.getLanguage).not.toHaveBeenCalled()
		expect(sdkLanguageMockState.addAppEventListen).not.toHaveBeenCalled()
	})
})
