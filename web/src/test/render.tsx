import type { RenderHookOptions, RenderOptions } from '@testing-library/react'
import type { ReactElement, ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, renderHook } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'

import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { UploadProgressToast } from '@/features/file-manager/components/upload-progress-toast'
import { createI18nInstance } from '@/i18n'

function createTestQueryClient() {
	return new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
			},
		},
	})
}

interface RenderWithProvidersOptions extends RenderOptions {
	queryClient?: QueryClient
}

interface RenderHookWithProvidersOptions<Props> extends RenderHookOptions<Props> {
	queryClient?: QueryClient
}

export function renderWithProviders(
	ui: ReactElement,
	{ queryClient = createTestQueryClient(), ...options }: RenderWithProvidersOptions = {},
) {
	function Wrapper({ children }: { children: ReactNode }) {
		const i18n = createI18nInstance('en')

		return (
			<QueryClientProvider client={queryClient}>
				<I18nextProvider i18n={i18n}>
					<TooltipProvider>{children}</TooltipProvider>
					<UploadProgressToast />
					<Toaster />
				</I18nextProvider>
			</QueryClientProvider>
		)
	}

	return {
		queryClient,
		...render(ui, { wrapper: Wrapper, ...options }),
	}
}

export function renderHookWithProviders<Result, Props>(
	renderHookCallback: (initialProps: Props) => Result,
	{ queryClient = createTestQueryClient(), ...options }: RenderHookWithProvidersOptions<Props> = {},
) {
	function Wrapper({ children }: { children: ReactNode }) {
		const i18n = createI18nInstance('en')

		return (
			<QueryClientProvider client={queryClient}>
				<I18nextProvider i18n={i18n}>
					<TooltipProvider>{children}</TooltipProvider>
					<UploadProgressToast />
					<Toaster />
				</I18nextProvider>
			</QueryClientProvider>
		)
	}

	return {
		queryClient,
		...renderHook(renderHookCallback, { wrapper: Wrapper, ...options }),
	}
}
