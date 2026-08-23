import type { FileBrowserSession, FileEntry } from '@/features/file-manager/types/file-manager'

export function editorLanguage(path: string) {
	const extension = path.split('.').pop()?.toLowerCase()
	switch (extension) {
		case 'css':
			return 'css'
		case 'html':
			return 'html'
		case 'js':
		case 'jsx':
			return 'javascript'
		case 'json':
			return 'json'
		case 'md':
			return 'markdown'
		case 'ts':
		case 'tsx':
			return 'typescript'
		case 'xml':
			return 'xml'
		case 'yaml':
		case 'yml':
			return 'yaml'
		default:
			return 'plaintext'
	}
}

export function formatFileModifiedTime(value: string) {
	if (!value) {
		return null
	}
	const date = new Date(value)
	if (Number.isNaN(date.getTime())) {
		return null
	}
	return {
		long: new Intl.DateTimeFormat(undefined, {
			dateStyle: 'full',
			timeStyle: 'long',
		}).format(date),
		short: formatRelativeTime(date, new Date()),
	}
}

export function formatRelativeTime(date: Date, now: Date) {
	const diffSeconds = Math.round((date.getTime() - now.getTime()) / 1000)
	const absoluteSeconds = Math.abs(diffSeconds)
	const units = [
		{ suffix: 's', seconds: 1 },
		{ suffix: 'm', seconds: 60 },
		{ suffix: 'h', seconds: 60 * 60 },
		{ suffix: 'd', seconds: 60 * 60 * 24 },
		{ suffix: 'mo', seconds: 60 * 60 * 24 * 30 },
		{ suffix: 'y', seconds: 60 * 60 * 24 * 365 },
	]
	const unit = [...units].reverse().find(item => absoluteSeconds >= item.seconds) ?? units[0]!
	const value = Math.max(0, Math.round(absoluteSeconds / unit.seconds))
	if (diffSeconds > 0) {
		return `in ${value}${unit.suffix}`
	}
	return `${value}${unit.suffix} ago`
}

export async function downloadEntry(session: FileBrowserSession, entry: FileEntry) {
	const blob = await session.client.downloadBlob(entry.path)
	const anchor = document.createElement('a')
	const url = URL.createObjectURL(blob)
	anchor.href = url
	anchor.download = entry.name
	anchor.rel = 'noreferrer'
	anchor.click()
	window.setTimeout(() => URL.revokeObjectURL(url), 0)
}

export function hasPendingBranches(branches: Record<string, { isLoading?: boolean } | undefined>) {
	return Object.values(branches).some(branch => branch?.isLoading)
}
