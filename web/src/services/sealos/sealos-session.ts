import type { SessionV1 } from '@labring/sealos-desktop-sdk'
import type { AppLocale } from '@/i18n'

interface SealosSessionUserExtras {
	language?: unknown
	locale?: unknown
	namespace?: unknown
}

export function resolveSealosUserNamespace(session: SessionV1 | null) {
	const user = session?.user as (SessionV1['user'] & SealosSessionUserExtras) | undefined
	const namespace = stringValue(user?.namespace)
	if (namespace) {
		return namespace
	}
	const k8sUsername = stringValue(user?.k8sUsername)
	if (k8sUsername?.startsWith('ns-')) {
		return k8sUsername
	}
	const nsid = stringValue(user?.nsid)
	if (nsid) {
		return nsid.startsWith('ns-') ? nsid : `ns-${nsid}`
	}
	return null
}

export function resolveSealosSessionLocale(session: SessionV1 | null): AppLocale | null {
	const user = session?.user as (SessionV1['user'] & SealosSessionUserExtras) | undefined
	const locale = stringValue(user?.locale) ?? stringValue(user?.language)
	if (locale === 'en' || locale === 'zh') {
		return locale
	}
	return null
}

function stringValue(value: unknown) {
	return typeof value === 'string' && value.trim().length > 0 ? value.trim() : null
}
