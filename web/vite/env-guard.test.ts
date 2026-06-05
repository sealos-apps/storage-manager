import { describe, expect, it } from 'vitest'

import { assertNoDevKubeconfigInBuild } from './env-guard'

describe('vite environment guard', () => {
	it('rejects production builds with the development kubeconfig env var', () => {
		expect(() => assertNoDevKubeconfigInBuild('build', 'apiVersion: v1')).toThrow(
			'VITE_DEV_KUBECONFIG is development-only',
		)
	})

	it('allows local dev with the development kubeconfig env var', () => {
		expect(() => assertNoDevKubeconfigInBuild('serve', 'apiVersion: v1')).not.toThrow()
	})

	it('allows production builds without the development kubeconfig env var', () => {
		expect(() => assertNoDevKubeconfigInBuild('build', undefined)).not.toThrow()
	})
})
