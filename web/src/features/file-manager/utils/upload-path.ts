import { normalizePath, parentPath } from '@sealos-storage-manager/filebrowser-client'

export interface UploadPathFile {
	name: string
	webkitRelativePath?: string
	relativePath?: string
}

export interface ResolvedUploadPath {
	directoryPaths: string[]
	relativePath: string
	parentPath: string
	fileName: string
	fullPath: string
}

/**
 * Normalizes a browser-provided relative path without allowing traversal.
 * Empty and current-directory segments are harmless and are removed.
 */
export function normalizeUploadRelativePath(path: string): string {
	const segments = splitUploadPath(path)
	if (segments.includes('..')) {
		throw new Error('Upload paths cannot contain parent-directory segments')
	}
	if (segments.length === 0) {
		throw new Error('Upload paths must contain a file name')
	}
	return segments.join('/')
}

export function uploadRelativePathForFile(file: UploadPathFile): string {
	return normalizeUploadRelativePath(file.webkitRelativePath || file.relativePath || file.name)
}

export function resolveUploadPath(targetDirectory: string, relativePath: string): ResolvedUploadPath {
	const normalizedTarget = normalizeUploadTargetDirectory(targetDirectory)
	const normalizedRelativePath = normalizeUploadRelativePath(relativePath)
	const fullPath = normalizedTarget === '/'
		? `/${normalizedRelativePath}`
		: `${normalizedTarget}/${normalizedRelativePath}`
	const directorySegments = normalizedRelativePath.split('/').slice(0, -1)
	const directoryPaths: string[] = []
	let directoryPath = normalizedTarget
	for (const segment of directorySegments) {
		directoryPath = directoryPath === '/' ? `/${segment}` : `${directoryPath}/${segment}`
		directoryPaths.push(directoryPath)
	}

	return {
		directoryPaths,
		fileName: normalizedRelativePath.slice(normalizedRelativePath.lastIndexOf('/') + 1),
		fullPath,
		parentPath: parentPath(fullPath),
		relativePath: normalizedRelativePath,
	}
}

function normalizeUploadTargetDirectory(path: string): string {
	const segments = splitUploadPath(path)
	if (segments.includes('..')) {
		throw new Error('Upload target paths cannot contain parent-directory segments')
	}
	return normalizePath(segments.join('/'))
}

function splitUploadPath(path: string): string[] {
	return path
		.replace(/\\/g, '/')
		.split('/')
		.filter(segment => segment && segment !== '.')
}
