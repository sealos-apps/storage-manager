import { describe, expect, it } from 'vitest'

import { assertNoDevOnlyEnvInBuild } from './env-guard'

describe('vite environment guard', () => {
	it('rejects production builds with the development kubeconfig env var', () => {
		expect(() => assertNoDevOnlyEnvInBuild('build', {
			devKubeconfig: 'apiVersion: v1',
		})).toThrow(
			'VITE_DEV_KUBECONFIG is development-only',
		)
	})

	it('rejects production builds with the development API base URL env var', () => {
		expect(() => assertNoDevOnlyEnvInBuild('build', {
			apiBaseUrl: 'http://localhost:4000',
		})).toThrow(
			'VITE_API_BASE_URL is development-only',
		)
	})

	it('rejects production builds with the development Desktop SDK bypass env var', () => {
		expect(() => assertNoDevOnlyEnvInBuild('build', {
			devDisableSealosDesktopSDK: 'true',
		})).toThrow(
			'VITE_DEV_DISABLE_SEALOS_DESKTOP_SDK is development-only',
		)
	})

	it('allows local dev with the development kubeconfig env var', () => {
		expect(() => assertNoDevOnlyEnvInBuild('serve', {
			apiBaseUrl: 'http://localhost:4000',
			devKubeconfig: 'apiVersion: v1',
			devDisableSealosDesktopSDK: 'true',
		})).not.toThrow()
	})

	it('allows production builds without development-only env vars', () => {
		expect(() => assertNoDevOnlyEnvInBuild('build', {})).not.toThrow()
	})
})
