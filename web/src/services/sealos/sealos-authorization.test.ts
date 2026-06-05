import type { SessionV1 } from '@labring/sealos-desktop-sdk'

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ViewerApiError } from '@/features/viewer/api/viewer-error'
import {
	getCachedAuthorizationHeader,
	getCachedSealosAuthorization,
	initializeSealosAuthorization,
	resetSealosAuthorizationForTest,
} from '@/services/sealos/sealos-authorization'

const sdk = vi.hoisted(() => ({
	createSealosApp: vi.fn(),
	getSession: vi.fn(),
}))

vi.mock('@labring/sealos-desktop-sdk/app', () => ({
	createSealosApp: sdk.createSealosApp,
	sealosApp: {
		getSession: sdk.getSession,
	},
}))

function sessionFixture(input: Partial<SessionV1> = {}): SessionV1 {
	return {
		kubeconfig: 'apiVersion: v1\nclusters: []',
		subscription: {
			CancelAt: '',
			CancelAtPeriodEnd: false,
			CreateAt: '',
			CurrentPeriodEndAt: '',
			CurrentPeriodStartAt: '',
			ExpireAt: null,
			ID: 'sub-1',
			PayMethod: '',
			PayStatus: '',
			PlanName: 'standard',
			RegionDomain: 'cloud.sealos.io',
			Status: 'active',
			Stripe: null,
			Traffic: null,
			TrafficStatus: '',
			UpdateAt: '',
			UserUID: 'user-1',
			Workspace: 'workspace-1',
			type: 'PAYG',
		},
		user: {
			avatar: '',
			id: 'user-1',
			k8sUsername: 'ns-admin',
			name: 'Admin',
			nsid: 'ns-1',
		},
		...input,
	}
}

describe('sealos authorization bootstrap', () => {
	let consoleWarnSpy: ReturnType<typeof vi.spyOn>

	beforeEach(() => {
		resetSealosAuthorizationForTest()
		vi.unstubAllEnvs()
		window.localStorage.clear()
		sdk.createSealosApp.mockReset()
		sdk.getSession.mockReset()
		consoleWarnSpy = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
	})

	afterEach(() => {
		resetSealosAuthorizationForTest()
		consoleWarnSpy.mockRestore()
		vi.unstubAllEnvs()
	})

	it('uses VITE_DEV_KUBECONFIG in dev, caches the header, and prints a CSS warning once', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_DEV_KUBECONFIG', 'apiVersion: v1\nclusters: []')

		await expect(initializeSealosAuthorization()).resolves.toMatchObject({
			authorizationHeader: 'Bearer apiVersion%3A%20v1%0Aclusters%3A%20%5B%5D',
			session: null,
			source: 'dev-kubeconfig',
		})
		await initializeSealosAuthorization()

		expect(getCachedAuthorizationHeader()).toBe('Bearer apiVersion%3A%20v1%0Aclusters%3A%20%5B%5D')
		expect(sdk.createSealosApp).not.toHaveBeenCalled()
		expect(sdk.getSession).not.toHaveBeenCalled()
		expect(consoleWarnSpy).toHaveBeenCalledTimes(1)
		expect(consoleWarnSpy).toHaveBeenCalledWith(
			expect.stringContaining('%c VITE_DEV_KUBECONFIG ACTIVE %c'),
			expect.stringContaining('background:'),
			expect.stringContaining('background:'),
		)
	})

	it('uses Sealos SDK in dev when VITE_DEV_KUBECONFIG is absent and prints safe auth info once', async () => {
		vi.stubEnv('DEV', true)
		sdk.createSealosApp.mockReturnValue(vi.fn())
		sdk.getSession.mockResolvedValue(sessionFixture())

		await expect(initializeSealosAuthorization()).resolves.toMatchObject({
			authorizationHeader: 'Bearer apiVersion%3A%20v1%0Aclusters%3A%20%5B%5D',
			source: 'sdk',
		})
		await initializeSealosAuthorization()

		expect(sdk.createSealosApp).toHaveBeenCalledTimes(1)
		expect(sdk.getSession).toHaveBeenCalledTimes(1)
		expect(consoleWarnSpy).toHaveBeenCalledTimes(1)
		expect(consoleWarnSpy).toHaveBeenCalledWith(
			expect.stringContaining('%c Sealos SDK auth ready '),
			expect.stringContaining('background:'),
			expect.objectContaining({
				k8sUsername: 'ns-admin',
				kubeconfigLength: 27,
				nsid: 'ns-1',
				userID: 'user-1',
			}),
		)
		expect(JSON.stringify(consoleWarnSpy.mock.calls)).not.toContain('apiVersion')
	})

	it('uses Sealos SDK in production and suppresses dev console auth info', async () => {
		vi.stubEnv('DEV', false)
		vi.stubEnv('VITE_DEV_KUBECONFIG', 'dev-kubeconfig')
		sdk.createSealosApp.mockReturnValue(vi.fn())
		sdk.getSession.mockResolvedValue(sessionFixture({ kubeconfig: 'prod-kubeconfig' }))

		await initializeSealosAuthorization()

		expect(getCachedAuthorizationHeader()).toBe('Bearer prod-kubeconfig')
		expect(consoleWarnSpy).not.toHaveBeenCalled()
	})

	it('ignores legacy authorization env and localStorage kubeconfig sources', async () => {
		vi.stubEnv('DEV', true)
		vi.stubEnv('VITE_VIEWER_AUTHORIZATION', 'Bearer legacy')
		window.localStorage.setItem('sealos-storage-manager.kubeconfig', 'stored-kubeconfig')
		sdk.createSealosApp.mockReturnValue(vi.fn())
		sdk.getSession.mockResolvedValue(sessionFixture({ kubeconfig: 'sdk-kubeconfig' }))

		await initializeSealosAuthorization()

		expect(getCachedAuthorizationHeader()).toBe('Bearer sdk-kubeconfig')
		expect(getCachedSealosAuthorization()?.source).toBe('sdk')
	})

	it('fails initialization when the SDK session does not include kubeconfig', async () => {
		vi.stubEnv('DEV', false)
		sdk.createSealosApp.mockReturnValue(vi.fn())
		sdk.getSession.mockResolvedValue(sessionFixture({ kubeconfig: '' }))

		await expect(initializeSealosAuthorization()).rejects.toMatchObject({
			code: 'UNAUTHORIZED',
			message: 'Sealos Desktop kubeconfig authorization is unavailable',
			status: 401,
		})
	})

	it('throws a business auth error when API code reads authorization before bootstrap', () => {
		expect(() => getCachedAuthorizationHeader()).toThrow(ViewerApiError)
		expect(() => getCachedAuthorizationHeader()).toThrow('Kubeconfig authorization has not been initialized')
	})
})
