import type { ViewerToken } from '@/features/viewer/types/viewer'

import { ExternalLink, KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

interface ActiveViewerFrameProps {
	token: ViewerToken | null
}

export function ActiveViewerFrame({ token }: ActiveViewerFrameProps) {
	const { t } = useTranslation()

	if (!token) {
		return (
			<Card className="rounded-lg">
				<CardHeader>
					<CardTitle>{t('viewer.viewerUrl')}</CardTitle>
					<CardDescription>{t('viewer.noSelection')}</CardDescription>
				</CardHeader>
			</Card>
		)
	}

	return (
		<Card className="min-h-[360px] rounded-lg">
			<CardHeader className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
				<div>
					<CardTitle>{t('viewer.viewerUrl')}</CardTitle>
					<CardDescription className="mt-2 break-all">{token.viewer_url}</CardDescription>
				</div>
				<Button asChild size="sm">
					<a href={token.viewer_url} rel="noreferrer" target="_blank">
						<ExternalLink />
						{t('actions.openViewer')}
					</a>
				</Button>
			</CardHeader>
			<CardContent>
				<div className="grid gap-4 md:grid-cols-[1fr_0.75fr]">
					<div className="flex min-h-[220px] items-center justify-center rounded-lg border bg-muted/40 text-center text-sm text-muted-foreground">
						<div className="max-w-md px-6">
							<KeyRound className="mx-auto mb-3 size-8" />
							<p>
								{t('actions.openViewer')}
								{' '}
								{t('viewer.activeSession')}
							</p>
							<p className="mt-2">
								{t('viewer.tokenExpires')}
								{': '}
								{token.expires_at}
							</p>
						</div>
					</div>
					<div className="rounded-lg border bg-background p-4">
						<div className="text-sm font-medium">{t('viewer.activeSession')}</div>
						<dl className="mt-3 flex flex-col gap-3 text-sm">
							<div>
								<dt className="text-muted-foreground">Viewer Session</dt>
								<dd className="break-all font-mono">{token.viewer_session_id}</dd>
							</div>
							<div>
								<dt className="text-muted-foreground">{t('viewer.podSession')}</dt>
								<dd className="break-all font-mono">{token.pod_session_id}</dd>
							</div>
							<div>
								<dt className="text-muted-foreground">Token Type</dt>
								<dd>{token.token_type}</dd>
							</div>
						</dl>
					</div>
				</div>
			</CardContent>
		</Card>
	)
}
