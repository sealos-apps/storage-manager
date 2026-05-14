import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'

import { en } from '@/i18n/locales/en'
import { zh } from '@/i18n/locales/zh'

export const resources = {
	en: { translation: en },
	zh: { translation: zh },
} as const

export type AppLocale = keyof typeof resources

export function createI18nInstance(locale: AppLocale = 'zh') {
	const instance = i18next.createInstance()
	void instance
		.use(initReactI18next)
		.init({
			fallbackLng: 'en',
			interpolation: {
				escapeValue: false,
			},
			lng: locale,
			resources,
		})
	return instance
}

export const i18n = createI18nInstance()
