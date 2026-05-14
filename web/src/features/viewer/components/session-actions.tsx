import type { ViewerAPI } from '@/features/viewer/types/viewer'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Power, ServerOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { viewerApi } from '@/features/viewer/api/viewer-api'
import { closePodSessionMutationOptions, closeViewerSessionMutationOptions } from '@/features/viewer/api/viewer-mutations'
import { viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'

interface SessionActionsProps {
	api?: ViewerAPI
	podSessionID: string | null
	viewerSessionID: string | null
}

export function SessionActions({
	api = viewerApi,
	podSessionID,
	viewerSessionID,
}: SessionActionsProps) {
	const queryClient = useQueryClient()
	const { t } = useTranslation()
	const closeViewer = useMutation(closeViewerSessionMutationOptions(queryClient, api))
	const closePod = useMutation(closePodSessionMutationOptions(queryClient, api))

	return (
		<div className="flex flex-col gap-2 sm:flex-row">
			<Button
				disabled={!viewerSessionID || closeViewer.isPending}
				onClick={() => {
					if (!viewerSessionID) {
						return
					}
					closeViewer.mutate(viewerSessionID, {
						onSuccess: () => viewerUIStore.actions.setActiveSession(null, null),
					})
				}}
				variant="outline"
			>
				<Power />
				{t('actions.closeViewer')}
			</Button>
			<Button
				disabled={!podSessionID || closePod.isPending}
				onClick={() => {
					if (!podSessionID) {
						return
					}
					closePod.mutate(podSessionID)
				}}
				variant="destructive"
			>
				<ServerOff />
				{t('actions.closePod')}
			</Button>
		</div>
	)
}
