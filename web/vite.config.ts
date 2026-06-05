import path from 'node:path'
import process from 'node:process'

import react from '@vitejs/plugin-react'
import { loadEnv } from 'vite'
import { defineConfig } from 'vitest/config'

import { encoreToolbar } from './vite/encore-toolbar'
import { assertNoDevOnlyEnvInBuild } from './vite/env-guard'

// https://vite.dev/config/
export default defineConfig(({ command, mode }) => {
	const viteEnv = loadEnv(mode, process.cwd(), 'VITE_')
	const apiBaseUrl = process.env.VITE_API_BASE_URL ?? viteEnv.VITE_API_BASE_URL
	const devKubeconfig = process.env.VITE_DEV_KUBECONFIG ?? viteEnv.VITE_DEV_KUBECONFIG
	assertNoDevOnlyEnvInBuild(command, {
		apiBaseUrl,
		devKubeconfig,
	})

	return {
		plugins: [react(), encoreToolbar()],
		resolve: {
			alias: {
				'@': path.resolve(__dirname, './src'),
				'@sealos-storage-manager/encore-client': path.resolve(__dirname, './packages/encore-client/src'),
				'@sealos-storage-manager/filebrowser-client': path.resolve(__dirname, './packages/filebrowser-client/src'),
			},
		},
		build: {
			// Chrome 86 is the minimum supported browser, not the only target.
			target: 'chrome86',
			cssTarget: 'chrome86',
		},
		test: {
			exclude: ['e2e/**', 'node_modules/**', 'dist/**'],
			environment: 'jsdom',
			globals: true,
			setupFiles: ['./src/test/setup.ts'],
			css: true,
		},
	}
})
