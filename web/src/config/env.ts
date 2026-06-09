const runtimeConfig = globalThis.window?.__SEALOS_STORAGE_MANAGER_CONFIG__
const apiBaseUrl = import.meta.env.DEV
	? import.meta.env.VITE_API_BASE_URL ?? runtimeConfig?.apiBaseUrl ?? '/api'
	: runtimeConfig?.apiBaseUrl ?? import.meta.env.VITE_API_BASE_URL ?? '/api'
const forcedLanguage = readForcedLanguage()
const disableSealosDesktopSDK = readDisableSealosDesktopSDK()

export const env = {
	apiBaseUrl,
	disableSealosDesktopSDK,
	forcedLanguage,
	fileUploadTusChunkBytes: parsePositiveInteger(
		runtimeConfig?.fileUploadTusChunkBytes,
		8 * 1024 * 1024,
	),
	fileUploadTusRetryCount: parsePositiveInteger(
		runtimeConfig?.fileUploadTusRetryCount,
		5,
	),
	fileUploadTusThresholdBytes: parsePositiveInteger(
		runtimeConfig?.fileUploadTusThresholdBytes,
		32 * 1024 * 1024,
	),
} as const

function parsePositiveInteger(
	value: number | string | undefined,
	fallback: number,
) {
	if (value === undefined || value === '') {
		return fallback
	}
	const parsed = typeof value === 'number' ? value : Number.parseInt(value, 10)
	return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : fallback
}

function parseBoolean(value: string | undefined) {
	return value === '1' || value === 'true'
}

export function readForcedLanguage() {
	return parseForcedLanguage(globalThis.window?.__SEALOS_STORAGE_MANAGER_CONFIG__?.forcedLanguage)
}

export function readDisableSealosDesktopSDK() {
	return import.meta.env.DEV && parseBoolean(import.meta.env.VITE_DEV_DISABLE_SEALOS_DESKTOP_SDK)
}

function parseForcedLanguage(value: string | undefined) {
	if (value === 'en' || value === 'zh') {
		return value
	}
	return ''
}
