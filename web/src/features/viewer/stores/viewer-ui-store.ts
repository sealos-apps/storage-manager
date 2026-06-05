import type { ViewerSelection } from '@/features/viewer/types/viewer'

import { useSelector } from '@tanstack/react-store'
import { createStore } from '@tanstack/store'

export type ViewerView = 'volumes' | 'files' | 'trash' | 'storageClasses'
export type Locale = 'en' | 'zh'

export interface ViewerUIState {
	activePodSessionID: string | null
	activeViewerSessionID: string | null
	locale: Locale
	namespace: string
	search: string
	selectedPVC: ViewerSelection | null
	view: ViewerView
}

export const initialViewerUIState: ViewerUIState = {
	activePodSessionID: null,
	activeViewerSessionID: null,
	locale: 'zh',
	namespace: '',
	search: '',
	selectedPVC: null,
	view: 'volumes',
}

export const viewerUIStore = createStore(initialViewerUIState, store => ({
	reset: () => store.setState(() => initialViewerUIState),
	selectPVC: (selectedPVC: ViewerSelection) =>
		store.setState(state => ({
			...state,
			selectedPVC,
			view: 'files',
		})),
	setActiveSession: (activeViewerSessionID: string | null, activePodSessionID: string | null) =>
		store.setState(state => ({
			...state,
			activePodSessionID,
			activeViewerSessionID,
		})),
	setLocale: (locale: Locale) =>
		store.setState(state => ({ ...state, locale })),
	setNamespace: (namespace: string) =>
		store.setState(state => ({
			...state,
			activePodSessionID: null,
			activeViewerSessionID: null,
			namespace,
			selectedPVC: null,
			view: 'volumes',
		})),
	syncContextNamespace: (namespace: string) =>
		store.setState((state) => {
			if (state.namespace === namespace) {
				return state
			}
			return {
				...state,
				activePodSessionID: null,
				activeViewerSessionID: null,
				namespace,
				selectedPVC: null,
				view: 'volumes',
			}
		}),
	setSearch: (search: string) =>
		store.setState(state => ({ ...state, search })),
	setView: (view: ViewerView) =>
		store.setState(state => ({ ...state, view })),
}))

export function useViewerNamespace() {
	return useSelector(viewerUIStore, state => state.namespace)
}

export function useViewerSearch() {
	return useSelector(viewerUIStore, state => state.search)
}

export function useViewerSelectedPVC() {
	return useSelector(viewerUIStore, state => state.selectedPVC)
}

export function useViewerView() {
	return useSelector(viewerUIStore, state => state.view)
}

export function useActiveViewerSessionID() {
	return useSelector(viewerUIStore, state => state.activeViewerSessionID)
}

export function useActivePodSessionID() {
	return useSelector(viewerUIStore, state => state.activePodSessionID)
}

export function useViewerLocale() {
	return useSelector(viewerUIStore, state => state.locale)
}
