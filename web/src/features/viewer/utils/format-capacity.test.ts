import { describe, expect, it } from 'vitest'

import { formatBytes } from '@/features/viewer/utils/format-capacity'

describe('formatBytes', () => {
	it('formats byte counts with binary units', () => {
		expect(formatBytes(0)).toBe('0 B')
		expect(formatBytes(512)).toBe('512 B')
		expect(formatBytes(1024)).toBe('1 KiB')
		expect(formatBytes(10 * 1024 ** 3)).toBe('10 GiB')
	})
})
