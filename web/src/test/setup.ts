import '@testing-library/jest-dom/vitest'

if (!Element.prototype.hasPointerCapture) {
	Element.prototype.hasPointerCapture = () => false
}

if (!Element.prototype.releasePointerCapture) {
	Element.prototype.releasePointerCapture = () => undefined
}

if (!Element.prototype.scrollIntoView) {
	Element.prototype.scrollIntoView = () => undefined
}

if (!Element.prototype.setPointerCapture) {
	Element.prototype.setPointerCapture = () => undefined
}

if (!window.matchMedia) {
	window.matchMedia = () =>
		({
			addEventListener: () => undefined,
			addListener: () => undefined,
			dispatchEvent: () => false,
			matches: false,
			media: '',
			onchange: null,
			removeEventListener: () => undefined,
			removeListener: () => undefined,
		}) as MediaQueryList
}

if (!window.ResizeObserver) {
	window.ResizeObserver = class ResizeObserver {
		disconnect() {}
		observe() {}
		unobserve() {}
	}
}
