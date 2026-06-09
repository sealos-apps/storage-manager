interface ImportMetaEnv {
	readonly VITE_API_BASE_URL?: string
	readonly VITE_DEV_DISABLE_SEALOS_DESKTOP_SDK?: string
	readonly VITE_DEV_KUBECONFIG?: string
}

interface ImportMeta {
	readonly env: ImportMetaEnv
}

interface SealosStorageManagerRuntimeConfig {
	readonly apiBaseUrl?: string
	readonly forcedLanguage?: string
	readonly fileUploadTusChunkBytes?: number | string
	readonly fileUploadTusRetryCount?: number | string
	readonly fileUploadTusThresholdBytes?: number | string
}

interface Window {
	__SEALOS_STORAGE_MANAGER_CONFIG__?: SealosStorageManagerRuntimeConfig
}
