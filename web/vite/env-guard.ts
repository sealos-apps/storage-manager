export function assertNoDevOnlyEnvInBuild(
	command: string,
	env: {
		apiBaseUrl?: string
		devDisableSealosDesktopSDK?: string
		devKubeconfig?: string
	},
) {
	if (command !== 'build') {
		return
	}
	if (env.devKubeconfig) {
		throw new Error('VITE_DEV_KUBECONFIG is development-only and must be removed before production build.')
	}
	if (env.devDisableSealosDesktopSDK) {
		throw new Error('VITE_DEV_DISABLE_SEALOS_DESKTOP_SDK is development-only and must be removed before production build.')
	}
	if (env.apiBaseUrl) {
		throw new Error('VITE_API_BASE_URL is development-only. Use runtime-config.js for production API root overrides.')
	}
}
