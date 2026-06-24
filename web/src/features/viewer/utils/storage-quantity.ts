import { BinaryScale, Quantity } from '@/utils/quantities'

const maxSafeInteger = BigInt(Number.MAX_SAFE_INTEGER)

export function parseGiQuantityInput(value: string) {
	const trimmed = value.trim()
	if (!/^\d+$/.test(trimmed)) {
		return null
	}
	const parsed = BigInt(trimmed)
	return parsed > 0n ? Quantity.newBinaryScaledQuantity(parsed, BinaryScale.Gibi) : null
}

export function quantityToSafeBytes(quantity: Quantity) {
	const value = quantity.value()
	if (value < 0n || value > maxSafeInteger) {
		return null
	}
	return Number(value)
}

export function capacityBytesToGiInput(quantity: Quantity) {
	const bytes = quantity.value()
	const gi = 1n << 30n
	return String(bytes <= 0n ? 1n : (bytes + gi - 1n) / gi)
}

export function formatQuantity(quantity: Quantity) {
	return quantity
		.formatForDisplay({ format: 'BinarySI', scale: 'auto', digits: 1 })
		.replace(/^(-?\d+(?:\.\d+)?)([a-z]+)$/i, '$1 $2B')
}

export function quantityPercent(used: Quantity, total: Quantity) {
	const totalBytes = total.value()
	if (totalBytes <= 0n) {
		return 0
	}
	const percent = (used.value() * 100n) / totalBytes
	return Math.min(100, Math.max(0, Number(percent)))
}

export function sumQuantities(items: Quantity[]) {
	return items.reduce((total, item) => total.add(item), Quantity.ZERO)
}

export function quantityFromBytes(bytes: number) {
	return Quantity.newQuantity(BigInt(Math.max(0, Math.trunc(bytes))), 'BinarySI')
}
