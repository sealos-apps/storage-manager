import type { ViewerAPI } from '@/features/viewer/types/viewer'
import type { ManualCloseKind } from '@/features/viewer/utils/session-capability'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Power, ServerOff } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { useHasActiveUploadsForSession } from '@/features/file-manager/stores/upload-store'
import { viewerApi } from '@/features/viewer/api/viewer-api'
import { closePodSessionMutationOptions, closeViewerSessionMutationOptions } from '@/features/viewer/api/viewer-mutations'
import { viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'

interface SessionActionsProps {
	api?: ViewerAPI
	canDiscardLocalState?: boolean
	onManualClose?: (kind: ManualCloseKind) => void
	podSessionID: string | null
	showPodAction?: boolean
	viewerSessionID: string | null
}

export function SessionActions({
	api = viewerApi,
	canDiscardLocalState = false,
	onManualClose,
	podSessionID,
	showPodAction = true,
	viewerSessionID,
}: SessionActionsProps) {
	const queryClient = useQueryClient()
	const { t } = useTranslation()
	const closeViewer = useMutation(closeViewerSessionMutationOptions(queryClient, api))
	const closePod = useMutation(closePodSessionMutationOptions(queryClient, api))
	const [blockedClose, setBlockedClose] = useState<ManualCloseKind | null>(null)
	const hasActiveUpload = useHasActiveUploadsForSession({
		podSessionID,
		viewerSessionID,
	})

	function showUploadGuard(kind: ManualCloseKind) {
		setBlockedClose(kind)
	}

	return (
		<>
			<div className="flex flex-col gap-2 sm:flex-row">
				<Button
					disabled={!viewerSessionID || closeViewer.isPending}
					onClick={() => {
						if (!viewerSessionID) {
							return
						}
						if (hasActiveUpload) {
							showUploadGuard('viewer')
							return
						}
						closeViewer.mutate(viewerSessionID, {
							onSuccess: () => {
								viewerUIStore.actions.setActiveSession(null, podSessionID)
								onManualClose?.('viewer')
							},
						})
					}}
					variant="outline"
				>
					<Power />
					{t('actions.closeViewer')}
				</Button>
				{showPodAction && podSessionID
					? (
							<Button
								disabled={closePod.isPending}
								onClick={() => {
									if (hasActiveUpload) {
										showUploadGuard('pod')
										return
									}
									closePod.mutate(podSessionID, {
										onSuccess: () => {
											viewerUIStore.actions.setActiveSession(null, null)
											onManualClose?.('pod')
										},
									})
								}}
								variant="destructive"
							>
								<ServerOff />
								{t('actions.closePod')}
							</Button>
						)
					: null}
				{showPodAction && canDiscardLocalState && !viewerSessionID && !podSessionID
					? (
							<Button
								onClick={() => {
									viewerUIStore.actions.setActiveSession(null, null)
									onManualClose?.('pod')
								}}
								variant="destructive"
							>
								<ServerOff />
								{t('actions.closePod')}
							</Button>
						)
					: null}
			</div>
			<Dialog onOpenChange={open => !open && setBlockedClose(null)} open={blockedClose !== null}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t('viewer.uploadGuardTitle')}</DialogTitle>
						<DialogDescription>
							{blockedClose === 'pod'
								? t('viewer.uploadGuardPodDescription')
								: t('viewer.uploadGuardViewerDescription')}
						</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button onClick={() => setBlockedClose(null)} type="button">
							{t('actions.close')}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	)
}
