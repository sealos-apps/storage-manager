import { describe, expect, it } from 'vitest'

import {
	capacityBytesToGiInput,
	formatQuantity,
	parseGiQuantityInput,
	quantityPercent,
	quantityToSafeBytes,
	sumQuantities,
} from '@/features/viewer/utils/storage-quantity'
import { Quantity } from '@/utils/quantities'

describe('storage quantity helpers', () => {
	it('parses positive integer Gi input', () => {
		const quantity = parseGiQuantityInput('10')

		expect(quantity?.toString()).toBe('10Gi')
		expect(quantityToSafeBytes(quantity ?? Quantity.ZERO)).toBe(10 * 1024 ** 3)
	})

	it('rejects empty, non-numeric, and zero Gi input', () => {
		expect(parseGiQuantityInput('')).toBeNull()
		expect(parseGiQuantityInput('1.5')).toBeNull()
		expect(parseGiQuantityInput('0')).toBeNull()
	})

	it('rounds bytes up to Gi input', () => {
		expect(capacityBytesToGiInput(Quantity.parse('1Gi'))).toBe('1')
		expect(capacityBytesToGiInput(Quantity.parse('1025Mi'))).toBe('2')
	})

	it('formats, sums, and compares quantities without number math', () => {
		const used = Quantity.parse('3Gi')
		const total = Quantity.parse('10Gi')

		expect(formatQuantity(total)).toBe('10 GiB')
		expect(quantityPercent(used, total)).toBe(30)
		expect(sumQuantities([used, total]).toString()).toBe('13Gi')
	})

	it('rejects unsafe byte serialization', () => {
		expect(quantityToSafeBytes(Quantity.newQuantity(BigInt(Number.MAX_SAFE_INTEGER) + 1n, 'BinarySI'))).toBeNull()
	})
})
