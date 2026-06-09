import type { SessionV1 } from '@labring/sealos-desktop-sdk'

import { describe, expect, it } from 'vitest'

import { resolveSealosSessionLocale, resolveSealosUserNamespace } from '@/services/sealos/sealos-session'

function sessionFixture(input: Partial<SessionV1> = {}): SessionV1 {
	return {
		kubeconfig: 'apiVersion: v1\nclusters: []',
		subscription: {} as SessionV1['subscription'],
		user: {
			avatar: '',
			id: 'user-1',
			k8sUsername: 'ns-admin',
			name: 'Admin',
			nsid: 'admin',
		},
		...input,
	}
}

describe('sealos session helpers', () => {
	it('resolves the user namespace from explicit namespace before nsid', () => {
		expect(resolveSealosUserNamespace(sessionFixture({
			user: {
				...sessionFixture().user,
				namespace: 'ns-explicit',
				nsid: 'fallback',
			} as SessionV1['user'],
		}))).toBe('ns-explicit')
	})

	it('resolves the user namespace from nsid', () => {
		expect(resolveSealosUserNamespace(sessionFixture())).toBe('ns-admin')
		expect(resolveSealosUserNamespace(sessionFixture({
			user: {
				...sessionFixture().user,
				k8sUsername: '',
				nsid: 'tenant',
			},
		}))).toBe('ns-tenant')
	})

	it('returns null when no namespace source is usable', () => {
		expect(resolveSealosUserNamespace(sessionFixture({
			user: {
				...sessionFixture().user,
				k8sUsername: '',
				nsid: '',
			},
		}))).toBeNull()
	})

	it('accepts only supported language values from the session', () => {
		expect(resolveSealosSessionLocale(sessionFixture({
			user: {
				...sessionFixture().user,
				locale: 'en',
			} as SessionV1['user'],
		}))).toBe('en')
		expect(resolveSealosSessionLocale(sessionFixture({
			user: {
				...sessionFixture().user,
				language: 'zh',
			} as SessionV1['user'],
		}))).toBe('zh')
		expect(resolveSealosSessionLocale(sessionFixture({
			user: {
				...sessionFixture().user,
				locale: 'fr',
			} as SessionV1['user'],
		}))).toBeNull()
	})
})
