import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import { StorageAppShell } from '@/features/viewer/components/storage-app-shell'
import { createStoragePolicyPrototypeAPI } from '@/features/viewer/prototype/storage-policy-api'
import { viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'
import { renderWithProviders } from '@/test/render'

beforeEach(() => {
	viewerUIStore.actions.reset()
	viewerUIStore.actions.syncContextNamespace('ns-admin')
	viewerUIStore.actions.setView('storageClasses')
})

async function editRow(name: string) {
	await userEvent.click(await screen.findByRole('button', { name: 'Storage types' }))
	const row = await screen.findByRole('row', { name: new RegExp(name) })
	await userEvent.click(within(row).getByRole('button', { name: 'Edit' }))
	return within(await screen.findByRole('dialog', { name: 'Edit storage type' }))
}

describe('storage policy prototype in the existing app shell', () => {
	it('saves localized names, mode and visibility through the existing query flow', async () => {
		const api = createStoragePolicyPrototypeAPI()
		renderWithProviders(<StorageAppShell api={api} />)
		const dialog = await editRow('Team files')
		const names = dialog.getAllByRole('textbox', { name: 'Display name' })
		const englishName = names.find(input => (input as HTMLInputElement).value === 'Team files')!
		await userEvent.clear(englishName)
		await userEvent.type(englishName, '研发共享盘')
		await userEvent.click(dialog.getByRole('radio', { name: /Exclusive/ }))
		await userEvent.click(dialog.getByRole('checkbox', { name: 'Available to users' }))
		await userEvent.click(dialog.getByRole('button', { name: 'Save' }))
		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
		expect(await screen.findByText('研发共享盘')).toBeInTheDocument()
		const saved = (await api.adminListStorageClasses()).find(item => item.name === 'nfs-team')!
		expect(saved).toMatchObject({ storage_mode: 'exclusive', available_to_users: true, display_names: { en: '研发共享盘', zh: '团队共享存储' } })
	})

	it('locks in-use mode and requires confirmation before hiding, with cancel preserving data', async () => {
		const api = createStoragePolicyPrototypeAPI()
		renderWithProviders(<StorageAppShell api={api} />)
		const dialog = await editRow('Performance disk')
		expect(dialog.getByRole('radio', { name: /Shared/ })).toBeDisabled()
		expect(dialog.getByRole('radio', { name: /Exclusive/ })).toBeDisabled()
		await userEvent.click(dialog.getByRole('checkbox', { name: 'Available to users' }))
		await userEvent.click(dialog.getByRole('button', { name: 'Save' }))
		let confirmation = within(await screen.findByRole('dialog', { name: 'Hide this storage type?' }))
		await userEvent.click(confirmation.getByRole('button', { name: 'Cancel' }))
		expect((await api.adminListStorageClasses())[0]?.available_to_users).toBe(true)
		await userEvent.click(dialog.getByRole('button', { name: 'Save' }))
		confirmation = within(await screen.findByRole('dialog', { name: 'Hide this storage type?' }))
		await userEvent.click(confirmation.getByRole('button', { name: 'Confirm hide' }))
		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
		expect((await api.adminListStorageClasses())[0]).toMatchObject({ available_to_users: false, storage_mode: 'exclusive', in_use_pvc_count: 128 })
	})

	it('rejects incomplete mode without saving and can cancel the draft', async () => {
		const api = createStoragePolicyPrototypeAPI()
		renderWithProviders(<StorageAppShell api={api} />)
		const dialog = await editRow('csi-archive')
		await userEvent.type(dialog.getByRole('textbox', { name: 'Display name' }), '归档存储')
		await userEvent.click(dialog.getByRole('checkbox', { name: 'Available to users' }))
		expect(dialog.getByRole('textbox', { name: 'Display name' })).toHaveValue('归档存储')
		await userEvent.click(dialog.getByRole('button', { name: 'Save' }))
		expect(await dialog.findByRole('alert')).toHaveTextContent('Select shared or exclusive storage.')
		expect(dialog.getByRole('textbox', { name: 'Display name' })).toHaveValue('归档存储')
		await userEvent.click(dialog.getByRole('button', { name: 'Cancel' }))
		expect((await api.adminListStorageClasses()).find(item => item.name === 'csi-archive')).toMatchObject({ available_to_users: false, display_names: {} })
	})

	it('isolates adapters and rejects unsupported resource writes', async () => {
		const api = createStoragePolicyPrototypeAPI()
		await api.adminUpdateStorageClassMetadata('nfs-team', { availableToUsers: true, displayNames: { zh: '新名字' }, storageMode: 'shared' })
		expect((await createStoragePolicyPrototypeAPI().adminListStorageClasses()).find(item => item.name === 'nfs-team')?.available_to_users).toBe(false)
		await expect(api.adminDeleteStorageClass('nfs-team')).rejects.toThrow('此原型仅演示')
		await expect(api.adminUpdateStorageClassMetadata('csi-rbd-sc', { availableToUsers: true, displayNames: { zh: '云盘' }, storageMode: 'shared' })).rejects.toThrow('使用中的存储属性')
	})
})
