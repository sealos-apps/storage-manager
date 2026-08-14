import { describe, expect, it } from 'vitest'

import {
	normalizeUploadRelativePath,
	resolveUploadPath,
	uploadRelativePathForFile,
} from '@/features/file-manager/utils/upload-path'

describe('upload path helpers', () => {
	it('normalizes browser relative paths and removes empty/current-directory segments', () => {
		expect(normalizeUploadRelativePath('project//./src\\index.ts')).toBe('project/src/index.ts')
		expect(normalizeUploadRelativePath('folder/ file.txt ')).toBe('folder/ file.txt ')
	})

	it('rejects parent-directory traversal in relative and target paths', () => {
		expect(() => normalizeUploadRelativePath('../secrets.txt')).toThrow(/parent-directory/)
		expect(() => normalizeUploadRelativePath('project/../../secrets.txt')).toThrow(/parent-directory/)
		expect(() => resolveUploadPath('/docs/../private', 'project/file.txt')).toThrow(/parent-directory/)
	})

	it('rejects an empty relative path', () => {
		expect(() => normalizeUploadRelativePath('/./')).toThrow(/file name/)
	})

	it('prefers webkitRelativePath, then relativePath, then the file name', () => {
		expect(uploadRelativePathForFile({
			name: 'fallback.txt',
			relativePath: 'relative/fallback.txt',
			webkitRelativePath: 'folder/file.txt',
		})).toBe('folder/file.txt')
		expect(uploadRelativePathForFile({
			name: 'fallback.txt',
			relativePath: 'relative/fallback.txt',
		})).toBe('relative/fallback.txt')
		expect(uploadRelativePathForFile({ name: 'fallback.txt' })).toBe('fallback.txt')
	})

	it('resolves the safe parent, file name, and full path under the target directory', () => {
		expect(resolveUploadPath('/docs', 'project//./src/file.txt')).toEqual({
			directoryPaths: ['/docs/project', '/docs/project/src'],
			fileName: 'file.txt',
			fullPath: '/docs/project/src/file.txt',
			parentPath: '/docs/project/src',
			relativePath: 'project/src/file.txt',
		})
		expect(resolveUploadPath('/', 'file.txt')).toEqual({
			directoryPaths: [],
			fileName: 'file.txt',
			fullPath: '/file.txt',
			parentPath: '/',
			relativePath: 'file.txt',
		})
		expect(resolveUploadPath('', './file.txt')).toEqual({
			directoryPaths: [],
			fileName: 'file.txt',
			fullPath: '/file.txt',
			parentPath: '/',
			relativePath: 'file.txt',
		})
	})
})
