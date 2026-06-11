import type { SessionV1 } from '@labring/sealos-desktop-sdk'

import { createSealosApp, sealosApp } from '@labring/sealos-desktop-sdk/app'

import { ViewerApiError } from '@/features/viewer/api/viewer-error'

export type SealosAuthorizationSource = 'dev-kubeconfig' | 'sdk'

export interface SealosAuthorizationState {
	accountAuthorizationHeader: string
	authorizationHeader: string
	session: SessionV1 | null
	source: SealosAuthorizationSource
}

interface OAuthTokenLike {
	access_token?: unknown
	token_type?: unknown
}

interface AccountSessionExtras {
	token?: unknown
}

let cachedAuthorization: SealosAuthorizationState | null = null
let initializePromise: Promise<SealosAuthorizationState> | null = null
let cleanupSealosApp: (() => void) | undefined
let devKubeconfigWarningPrinted = false
let sdkAuthInfoPrinted = false

export function getCachedAuthorizationHeader() {
	if (!cachedAuthorization) {
		throw new ViewerApiError({
			code: 'UNAUTHORIZED',
			message: 'Kubeconfig authorization has not been initialized',
			status: 401,
		})
	}
	return cachedAuthorization.authorizationHeader
}

export function getCachedAccountAuthorizationHeader() {
	return cachedAuthorization?.accountAuthorizationHeader ?? ''
}

export function getCachedSealosAuthorization() {
	return cachedAuthorization
}

export async function initializeSealosAuthorization() {
	if (cachedAuthorization) {
		return cachedAuthorization
	}
	if (initializePromise) {
		return initializePromise
	}

	initializePromise = resolveSealosAuthorization()
		.then((authorization) => {
			cachedAuthorization = authorization
			return authorization
		})
		.finally(() => {
			initializePromise = null
		})

	return initializePromise
}

export function resetSealosAuthorizationForTest() {
	cachedAuthorization = null
	initializePromise = null
	cleanupSealosApp?.()
	cleanupSealosApp = undefined
	devKubeconfigWarningPrinted = false
	sdkAuthInfoPrinted = false
}

async function resolveSealosAuthorization(): Promise<SealosAuthorizationState> {
	const devKubeconfig = import.meta.env.DEV ? import.meta.env.VITE_DEV_KUBECONFIG : undefined
	if (devKubeconfig) {
		printDevKubeconfigWarning()
		return {
			accountAuthorizationHeader: '',
			authorizationHeader: encodeKubeconfigAuthorization(devKubeconfig),
			session: null,
			source: 'dev-kubeconfig',
		}
	}

	try {
		cleanupSealosApp ??= createSealosApp()
		const session = await sealosApp.getSession()
		if (!session.kubeconfig) {
			throw new Error('Sealos Desktop session did not include kubeconfig')
		}
		printSdkAuthorizationInfo(session)
		return {
			accountAuthorizationHeader: resolveAccountAuthorizationHeader(session),
			authorizationHeader: encodeKubeconfigAuthorization(session.kubeconfig),
			session,
			source: 'sdk',
		}
	}
	catch (error) {
		throw new ViewerApiError({
			code: 'UNAUTHORIZED',
			details: {
				source: 'sealos-sdk',
				message: error instanceof Error ? error.message : String(error),
			},
			message: 'Sealos Desktop kubeconfig authorization is unavailable',
			status: 401,
		})
	}
}

function encodeKubeconfigAuthorization(kubeconfig: string) {
	return `Bearer ${encodeURIComponent(kubeconfig)}`
}

function resolveAccountAuthorizationHeader(session: SessionV1) {
	const token = (session as SessionV1 & AccountSessionExtras).token
	if (typeof token === 'string' && token.trim().length > 0) {
		return normalizeBearerToken(token)
	}
	const accessToken = stringValue((token as OAuthTokenLike | undefined)?.access_token)
	if (!accessToken) {
		return ''
	}
	const tokenType = stringValue((token as OAuthTokenLike | undefined)?.token_type) ?? 'Bearer'
	return `${tokenType} ${accessToken}`
}

function normalizeBearerToken(token: string) {
	const trimmed = token.trim()
	return /^Bearer\s+/i.test(trimmed) ? trimmed : `Bearer ${trimmed}`
}

function stringValue(value: unknown) {
	return typeof value === 'string' && value.trim().length > 0 ? value.trim() : null
}

function printDevKubeconfigWarning() {
	if (devKubeconfigWarningPrinted) {
		return
	}
	devKubeconfigWarningPrinted = true
	console.warn(
		'%c VITE_DEV_KUBECONFIG ACTIVE %c development only - production build is blocked ',
		'background:#7f1d1d;color:#fef2f2;font-weight:700;padding:3px 6px;border-radius:4px;',
		'background:#fef3c7;color:#78350f;font-weight:700;padding:3px 6px;border-radius:4px;',
	)
}

function printSdkAuthorizationInfo(session: SessionV1) {
	if (!import.meta.env.DEV || sdkAuthInfoPrinted) {
		return
	}
	sdkAuthInfoPrinted = true
	console.warn(
		'%c Sealos SDK auth ready ',
		'background:#064e3b;color:#ecfdf5;font-weight:700;padding:3px 6px;border-radius:4px;',
		{
			k8sUsername: session.user.k8sUsername,
			kubeconfigLength: session.kubeconfig.length,
			nsid: session.user.nsid,
			userID: session.user.id,
		},
	)
}
