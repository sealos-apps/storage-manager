import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { summarizeUploadBatches, useUploadTasks } from '@/features/file-manager/stores/upload-store'

function toastID(batchID: string) {
	return `upload-progress:${batchID}`
}

export function UploadProgressToast() {
	const { t } = useTranslation()
	const tasks = useUploadTasks()
	const summaries = summarizeUploadBatches(tasks)
	const previousBatchIDsRef = useRef(new Set<string>())
	const lastMessagesRef = useRef(new Map<string, string>())
	const settledBatchIDsRef = useRef(new Set<string>())

	useEffect(() => {
		const currentBatchIDs = new Set(summaries.map(summary => summary.batchID))
		for (const batchID of previousBatchIDsRef.current) {
			if (currentBatchIDs.has(batchID)) {
				continue
			}
			toast.dismiss(toastID(batchID))
			lastMessagesRef.current.delete(batchID)
			settledBatchIDsRef.current.delete(batchID)
		}
		previousBatchIDsRef.current = currentBatchIDs

		for (const summary of summaries) {
			const id = toastID(summary.batchID)
			const message = summary.failed > 0
				? t('files.uploadProgressWithFailures', {
						failed: summary.failed,
						succeeded: summary.uploaded,
					})
				: t('files.uploadProgress', { count: summary.uploaded })

			if (summary.isComplete) {
				if (settledBatchIDsRef.current.has(summary.batchID)) {
					continue
				}
				settledBatchIDsRef.current.add(summary.batchID)
				lastMessagesRef.current.delete(summary.batchID)
				if (summary.failed > 0) {
					toast.error(message, { duration: 8000, id })
				}
				else {
					toast.success(message, { duration: 5000, id })
				}
				continue
			}

			if (lastMessagesRef.current.get(summary.batchID) === message) {
				continue
			}
			lastMessagesRef.current.set(summary.batchID, message)
			toast.loading(message, { duration: Infinity, id })
		}
	}, [summaries, t])

	return null
}
