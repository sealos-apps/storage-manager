import type { UseMutationResult } from '@tanstack/react-query'

import type { PVC, StorageClass } from '@/features/viewer/types/viewer'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { describe, expect, it, vi } from 'vitest'
import { DeleteStorageClassDialog } from '@/features/viewer/components/storage-class-dialogs'
import { DeletePVCDialog } from '@/features/viewer/components/volume-dialogs'
import { pvcFixture } from '@/features/viewer/test/fakes'
import { renderWithProviders } from '@/test/render'

function mutation<T, Variables>(mutate = vi.fn()): UseMutationResult<T, Error, Variables> {
	return {
		isPending: false,
		mutate,
	} as unknown as UseMutationResult<T, Error, Variables>
}

describe('delete dialogs', () => {
	it('enables PVC deletion after the exact PVC name is typed', async () => {
		const user = userEvent.setup()
		const mutate = vi.fn()
		const pvc = pvcFixture({ name: 'mysql-data' })

		renderWithProviders(
			<DeletePVCDialog
				deleteState={{ confirmName: '', pvc }}
				mutation={mutation<PVC, { name: string, namespace: string }>(mutate)}
				onOpenChange={vi.fn()}
				onSuccess={vi.fn()}
			/>,
		)

		expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()

		await user.type(screen.getByLabelText('Type PVC name to confirm'), 'mysql-data')

		expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled()
	})

	it('renders the PVC delete name as bold selectable text', () => {
		const pvc = pvcFixture({ name: 'mysql-data' })

		renderWithProviders(
			<DeletePVCDialog
				deleteState={{ confirmName: '', pvc }}
				mutation={mutation<PVC, { name: string, namespace: string }>()}
				onOpenChange={vi.fn()}
				onSuccess={vi.fn()}
			/>,
		)

		const name = screen.getByText('mysql-data')
		expect(name.tagName).toBe('STRONG')
		expect(name).toHaveClass('select-all')
		expect(name.closest('p')).toHaveTextContent('Type mysql-data to confirm deletion. Mounted PVCs cannot be deleted.')
	})

	it('renders the storage class delete name as bold selectable text', () => {
		renderWithProviders(
			<DeleteStorageClassDialog
				mutation={mutation<StorageClass, string>()}
				name="standard"
				onOpenChange={vi.fn()}
			/>,
		)

		const name = screen.getByText('standard')
		expect(name.tagName).toBe('STRONG')
		expect(name).toHaveClass('select-all')
		expect(name.closest('p')).toHaveTextContent('Type standard to confirm deletion. This deletes the storage type from the cluster.')
		expect(screen.getByLabelText('Type storage type name to confirm')).toBeInTheDocument()
	})
})
