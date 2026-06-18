import type { FileBrowserResource } from '@sealos-storage-manager/filebrowser-client'
import type { QueryClient } from '@tanstack/react-query'
import type { ComponentProps } from 'react'
import type { FileBrowserSession } from '@/features/file-manager/types/file-manager'
import type { PVC } from '@/features/viewer/types/viewer'

import { vi } from 'vitest'

import { FileManagerView } from '@/features/file-manager/components/file-manager-view'
import { pvcFixture, viewerSessionFixture, viewerTokenFixture } from '@/features/viewer/test/fakes'
import { deriveSessionCapability } from '@/features/viewer/utils/session-capability'
import { renderWithProviders } from '@/test/render'

vi.mock('@/components/monaco-editor', () => ({
	default: ({
		onChange,
		value,
	}: {
		onChange?: (value?: string) => void
		value?: string
	}) => (
		<textarea
			aria-label="Monaco editor"
			onChange={event => onChange?.(event.target.value)}
			value={value ?? ''}
		/>
	),
}))

export function resource(path: string, name: string, isDir: boolean, items?: FileBrowserResource[]): FileBrowserResource {
	return {
		isDir,
		items,
		modified: '2026-05-14T10:00:00Z',
		name,
		path,
		size: isDir ? 0 : 12,
	}
}

export function readyCapability() {
	return deriveSessionCapability({
		error: null,
		isReconnecting: false,
		manualCloseKind: null,
		selectedPVC: pvcFixture(),
		session: viewerSessionFixture({ status: 'ready', token_ready: true }),
		status: 'ready',
		token: viewerTokenFixture(),
	})
}

export function reconnectingCapability() {
	return deriveSessionCapability({
		error: null,
		isReconnecting: true,
		manualCloseKind: null,
		selectedPVC: pvcFixture(),
		session: viewerSessionFixture({ status: 'ready', token_ready: true }),
		status: 'failed',
		token: null,
	})
}

export interface RenderFileManagerOptions {
	api?: ComponentProps<typeof FileManagerView>['api']
	currentPath?: string
	onManualClose?: ComponentProps<typeof FileManagerView>['onManualClose']
	onPathChange?: (path: string) => void
	onRefreshSession?: () => void
	onRefreshStorageData?: () => void
	podSessionID?: string | null
	pvc?: PVC | null
	queryClient?: QueryClient
	sessionCapability?: ComponentProps<typeof FileManagerView>['sessionCapability']
	viewerSession?: ComponentProps<typeof FileManagerView>['viewerSession']
	viewerSessionID?: string | null
}

export function renderFileManager(
	session: FileBrowserSession | null,
	currentPathOrOptions: string | RenderFileManagerOptions = '/',
	onPathChange = vi.fn(),
) {
	const options: RenderFileManagerOptions = typeof currentPathOrOptions === 'string'
		? { currentPath: currentPathOrOptions, onPathChange }
		: currentPathOrOptions
	const capability = options.sessionCapability ?? (session
		? readyCapability()
		: deriveSessionCapability({
				error: null,
				isReconnecting: false,
				manualCloseKind: null,
				selectedPVC: pvcFixture(),
				session: viewerSessionFixture({ status: 'creating', token_ready: false }),
				status: 'polling',
				token: null,
			}))

	return renderWithProviders(
		<FileManagerView
			api={options.api}
			currentPath={options.currentPath ?? '/'}
			onBackToVolumes={vi.fn()}
			onManualClose={options.onManualClose}
			onPathChange={options.onPathChange ?? vi.fn()}
			onRefreshSession={options.onRefreshSession ?? vi.fn()}
			onRefreshStorageData={options.onRefreshStorageData ?? vi.fn()}
			podSessionID={options.podSessionID}
			pvc={options.pvc ?? pvcFixture()}
			pvcName="data"
			session={session}
			sessionCapability={capability}
			setSort={vi.fn()}
			sort={{ field: 'name', direction: 'asc' }}
			viewerSession={options.viewerSession}
			viewerSessionID={options.viewerSessionID}
		/>,
		{ queryClient: options.queryClient },
	)
}

export function sessionWithClient(client: Record<string, unknown>): FileBrowserSession {
	return {
		client: {
			usage: vi.fn().mockResolvedValue({ total: 20 * 1024 * 1024 * 1024, used: 5 * 1024 * 1024 * 1024 }),
			...client,
		},
		pvcKey: 'pvc-1',
	} as unknown as FileBrowserSession
}
