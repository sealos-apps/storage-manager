import type { ReactNode } from 'react'
import type { AppLocale } from '@/i18n'

import { EVENT_NAME } from '@labring/sealos-desktop-sdk'
import { sealosApp } from '@labring/sealos-desktop-sdk/app'
import { useEffect, useState } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { readDisableSealosDesktopSDK, readForcedLanguage } from '@/config/env'
import { i18n } from '@/i18n'
import { initializeSealosAuthorization } from '@/services/sealos/sealos-authorization'
import { resolveSealosSessionLocale } from '@/services/sealos/sealos-session'

interface AuthBootstrapProps {
	children: ReactNode
}

type AuthBootstrapState
	= | { status: 'loading' }
		| { message: string, status: 'error' }
		| { status: 'ready' }

export function AuthBootstrap({ children }: AuthBootstrapProps) {
	const [state, setState] = useState<AuthBootstrapState>({ status: 'loading' })

	useEffect(() => {
		let mounted = true
		let unsubscribeLanguage: (() => void) | undefined

		initializeSealosAuthorization()
			.then(async (authorization) => {
				const forcedLanguage = readForcedLanguage()
				const disableSealosDesktopSDK = readDisableSealosDesktopSDK()
				const sessionLocale = resolveSealosSessionLocale(authorization.session)
				const locale = forcedLanguage || (disableSealosDesktopSDK ? sessionLocale : await resolveInitialDesktopLocale(sessionLocale))
				if (locale) {
					await i18n.changeLanguage(locale)
				}
				if (!forcedLanguage && !disableSealosDesktopSDK) {
					unsubscribeLanguage = sealosApp.addAppEventListen(EVENT_NAME.CHANGE_I18N, (data: unknown) => {
						const nextLocale = resolveDesktopLanguageEventLocale(data)
						if (nextLocale && nextLocale !== i18n.language) {
							void i18n.changeLanguage(nextLocale)
						}
					})
				}
				if (mounted) {
					setState({ status: 'ready' })
				}
			})
			.catch((error: unknown) => {
				if (mounted) {
					setState({
						message: error instanceof Error ? error.message : 'Failed to initialize Sealos authorization',
						status: 'error',
					})
				}
			})

		return () => {
			mounted = false
			unsubscribeLanguage?.()
		}
	}, [])

	if (state.status === 'ready') {
		return children
	}

	if (state.status === 'error') {
		return (
			<main className="flex min-h-screen items-center justify-center bg-muted/30 px-6 py-10 text-foreground">
				<Alert className="max-w-lg" variant="destructive">
					<AlertTitle>Authorization unavailable</AlertTitle>
					<AlertDescription className="mt-2 space-y-4">
						<p>{state.message}</p>
						<Button onClick={() => globalThis.location.reload()} type="button" variant="outline">
							Retry
						</Button>
					</AlertDescription>
				</Alert>
			</main>
		)
	}

	return (
		<main className="flex min-h-screen items-center justify-center bg-muted/30 px-6 py-10 text-foreground">
			<div className="space-y-3 text-center">
				<div className="mx-auto size-10 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-foreground" />
				<p className="text-sm text-muted-foreground">Initializing Sealos authorization...</p>
			</div>
		</main>
	)
}

async function resolveInitialDesktopLocale(fallback: AppLocale | null) {
	try {
		const language = await sealosApp.getLanguage()
		return normalizeLocale(language.lng) ?? fallback
	}
	catch {
		return fallback
	}
}

function resolveDesktopLanguageEventLocale(data: unknown) {
	if (typeof data !== 'object' || data === null) {
		return null
	}
	const currentLanguage = (data as { currentLanguage?: unknown }).currentLanguage
	return normalizeLocale(currentLanguage)
}

function normalizeLocale(value: unknown): AppLocale | null {
	return value === 'en' || value === 'zh' ? value : null
}
