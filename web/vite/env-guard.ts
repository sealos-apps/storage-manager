export function assertNoDevKubeconfigInBuild(command: string, devKubeconfig: string | undefined) {
	if (command === 'build' && devKubeconfig) {
		throw new Error('VITE_DEV_KUBECONFIG is development-only and must be removed before production build.')
	}
}
