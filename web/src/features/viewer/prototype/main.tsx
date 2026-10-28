import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { viewerKeys } from '@/features/viewer/api/viewer-query-keys'
import { StorageAppShell } from '@/features/viewer/components/storage-app-shell'
import { createStoragePolicyPrototypeAPI } from '@/features/viewer/prototype/storage-policy-api'
import { viewerUIStore } from '@/features/viewer/stores/viewer-ui-store'
import { createI18nInstance } from '@/i18n'
import '@/polyfills'
import '@/styles/globals.css'

const root = document.getElementById('root')
if (!root) {
	throw new Error('Missing prototype root')
}
const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
const api = createStoragePolicyPrototypeAPI()
viewerUIStore.actions.syncContextNamespace('ns-admin')
viewerUIStore.actions.setView('storageClasses')

const reactRoot = createRoot(root)
void Promise.all([api.adminCapabilities(), api.getContext()]).then(([capabilities, context]) => {
	client.setQueryData(viewerKeys.adminCapabilities(), capabilities)
	client.setQueryData(viewerKeys.context(), context)
	reactRoot.render(
		<QueryClientProvider client={client}>
			<I18nextProvider i18n={createI18nInstance('zh')}>
				<TooltipProvider>
					<div className="border-b bg-muted px-4 py-2 text-center text-xs text-muted-foreground">
						存储类型配置原型 · 复用项目现有页面 · 示例数据仅保存在内存，刷新页面重置
					</div>
					<StorageAppShell api={api} />
					<Toaster />
				</TooltipProvider>
			</I18nextProvider>
		</QueryClientProvider>,
	)
})
