import { expect, test } from '@playwright/test'

const pvc = {
	access_modes: ['ReadWriteOnce'],
	capacity: '10Gi',
	capacity_bytes: 10 * 1024 * 1024 * 1024,
	mounted: true,
	mounted_pods: [
		{
			name: 'mysql-0',
			namespace: 'default',
			node_name: 'node-a',
			phase: 'Running',
			read_only: false,
		},
	],
	name: 'mysql-data',
	namespace: 'default',
	reason: '',
	uid: 'pvc-uid',
	viewer_mode: 'readwrite',
	viewer_scheduling: {
		node_name: 'node-a',
		reason: 'ReadWriteOnce PVC is already mounted on node-a',
		requires_node: true,
	},
	viewer_supported: true,
}

test.beforeEach(async ({ page }) => {
	await page.addInitScript(() => {
		window.localStorage.setItem('sealos-storage-manager.kubeconfig', 'test-kubeconfig')
	})
})

test('launches a File Browser viewer session from the PVC list', async ({ page }) => {
	await page.route('http://localhost:4000/api/pvcs?namespace=default', async (route) => {
		await route.fulfill({
			contentType: 'application/json',
			json: { pvc_list: { items: [pvc] } },
		})
	})
	await page.route('http://localhost:4000/api/viewer-sessions', async (route) => {
		await route.fulfill({
			contentType: 'application/json',
			json: {
				viewer_session: {
					created_at: '2026-05-14T10:00:00Z',
					expires_at: '2026-05-14T10:03:00Z',
					id: 'vs_1',
					last_heartbeat_at: '2026-05-14T10:00:00Z',
					mode: 'readwrite',
					pod_session_id: 'ps_1',
					pod_status: 'ready',
					reason: '',
					status: 'ready',
					token_ready: true,
					viewer_url: 'https://viewer.example.test',
				},
			},
		})
	})
	await page.route('http://localhost:4000/api/viewer-sessions/vs_1/token', async (route) => {
		await route.fulfill({
			contentType: 'application/json',
			headers: {
				'Cache-Control': 'no-store',
				'Pragma': 'no-cache',
			},
			json: {
				viewer_token: {
					expires_at: '2026-05-14T10:30:00Z',
					pod_session_id: 'ps_1',
					token: 'fb-token',
					token_type: 'Bearer',
					viewer_session_id: 'vs_1',
					viewer_url: 'https://viewer.example.test',
				},
			},
		})
	})
	await page.route('http://localhost:4000/api/viewer-sessions/vs_1/heartbeat', async (route) => {
		await route.fulfill({
			contentType: 'application/json',
			json: {
				heartbeat: {
					expires_at: '2026-05-14T10:03:00Z',
					server_time: '2026-05-14T10:00:20Z',
					status: 'active',
					viewer_session_id: 'vs_1',
				},
			},
		})
	})

	await page.goto('/')

	await expect(page.getByText('mysql-data')).toBeVisible()
	await page.getByRole('button', { name: '打开 Viewer' }).click()
	await expect(page.getByText('https://viewer.example.test')).toBeVisible()
	await expect(page.getByRole('link', { name: '打开 File Browser' })).toHaveAttribute('href', 'https://viewer.example.test')
})

test('shows localized backend errors and can switch locale', async ({ page }) => {
	await page.route('http://localhost:4000/api/pvcs?namespace=default', async (route) => {
		await route.fulfill({
			contentType: 'application/json',
			json: {
				code: 'permission_denied',
				details: { Code: 'PVC_ACCESS_DENIED' },
				message: 'PVC access denied',
			},
			status: 403,
		})
	})

	await page.goto('/')
	await expect(page.getByText(/没有权限访问/)).toBeVisible()
	await page.getByRole('button', { name: 'Locale' }).click()
	await expect(page.getByText(/permission to access/)).toBeVisible()
})
