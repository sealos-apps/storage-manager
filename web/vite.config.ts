import path from 'node:path'

import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

import { encoreToolbar } from './vite/encore-toolbar'

// https://vite.dev/config/
export default defineConfig({
	plugins: [react(), encoreToolbar()],
	resolve: {
		alias: {
			'@': path.resolve(__dirname, './src'),
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
})
