interface ImportMetaEnv {
	readonly VITE_API_BASE_URL?: string
	readonly VITE_APP_NAME?: string
	readonly VITE_DEV_KUBECONFIG?: string
	readonly VITE_FILE_UPLOAD_TUS_CHUNK_BYTES?: string
	readonly VITE_FILE_UPLOAD_TUS_RETRY_COUNT?: string
	readonly VITE_FILE_UPLOAD_TUS_THRESHOLD_BYTES?: string
}

interface ImportMeta {
	readonly env: ImportMetaEnv
}
