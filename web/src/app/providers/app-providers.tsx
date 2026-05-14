import type { ReactNode } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'

import { I18nProvider } from '@/app/providers/i18n-provider'
import { TanStackDevtoolsPanel } from '@/app/providers/tanstack-devtools-panel'
import { TooltipProvider } from '@/components/ui/tooltip'
import { queryClient } from '@/services/query-client'

interface AppProvidersProps {
	children: ReactNode
}

export function AppProviders({ children }: AppProvidersProps) {
	return (
		<QueryClientProvider client={queryClient}>
			<I18nProvider>
				<TooltipProvider>{children}</TooltipProvider>
			</I18nProvider>
			<ReactQueryDevtools buttonPosition="bottom-left" initialIsOpen={false} />
			<TanStackDevtoolsPanel />
		</QueryClientProvider>
	)
}
