import type { ReactNode } from 'react'

import { useEffect, useState } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { initializeSealosAuthorization } from '@/services/sealos/sealos-authorization'

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

		initializeSealosAuthorization()
			.then(() => {
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
