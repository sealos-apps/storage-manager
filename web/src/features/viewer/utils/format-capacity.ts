export function formatBytes(bytes: number) {
	if (!Number.isFinite(bytes) || bytes <= 0) {
		return '0 B'
	}
	const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
	let value = bytes
	let unitIndex = 0
	while (value >= 1024 && unitIndex < units.length - 1) {
		value /= 1024
		unitIndex += 1
	}
	const rounded = value >= 10 || unitIndex === 0 ? Math.round(value) : Number(value.toFixed(1))
	return `${rounded} ${units[unitIndex]}`
}
