import postcssIsPseudoClass from '@csstools/postcss-is-pseudo-class'
import tailwindcss from '@tailwindcss/postcss'
import postcssPresetEnv from 'postcss-preset-env'
import postcssTransformShortcut from 'postcss-transform-shortcut'
import valueParser from 'postcss-value-parser'

function fixEmptyCssVariableFallbacks() {
	return {
		postcssPlugin: 'fix-empty-css-variable-fallbacks',
		Declaration(declaration) {
			if (declaration.value.includes(',)')) {
				declaration.value = declaration.value.replaceAll(',)', ', )')
			}
		},
	}
}

fixEmptyCssVariableFallbacks.postcss = true

function resolveRootVariablesInColorMix() {
	return {
		postcssPlugin: 'resolve-root-variables-in-color-mix',
		Once(root) {
			const rootVariables = new Map()

			root.walkRules(':root', (rule) => {
				rule.walkDecls((declaration) => {
					if (declaration.prop.startsWith('--')) {
						rootVariables.set(declaration.prop, declaration.value)
					}
				})
			})

			root.walkDecls((declaration) => {
				if (!declaration.value.includes('color-mix(')) {
					return
				}

				const parsedValue = valueParser(declaration.value)

				parsedValue.walk((node) => {
					if (node.type !== 'function' || node.value !== 'var') {
						return
					}

					const variableName = valueParser.stringify(node.nodes).trim()
					const variableValue = rootVariables.get(variableName)

					if (!variableValue) {
						return
					}

					node.type = 'word'
					node.value = variableValue
					node.nodes = undefined
				})

				declaration.value = parsedValue.toString()
			})
		},
	}
}

resolveRootVariablesInColorMix.postcss = true

function fallbackGradientColorSpaces() {
	return {
		postcssPlugin: 'fallback-gradient-color-spaces',
		Declaration(declaration) {
			if (!declaration.value.includes(' in oklab')) {
				return
			}

			declaration.cloneBefore({
				value: declaration.value.replaceAll(' in oklab', ''),
			})
		},
	}
}

fallbackGradientColorSpaces.postcss = true

function flattenTranslucentColorMixFallbacks() {
	return {
		postcssPlugin: 'flatten-translucent-color-mix-fallbacks',
		OnceExit(root, { result }) {
			const fallbackColors = new Map()
			const unhandledFallbacks = new Map()
			const replaced = new Set()

			root.walkAtRules('supports', (atRule) => {
				if (!atRule.params.includes('color-mix(')) {
					return
				}

				atRule.walkRules((rule) => {
					if (rule.parent !== atRule) {
						return
					}

					for (const declaration of rule.nodes) {
						const rgba = colorMixRgbToRgba(declaration.value)
						if (!rgba) {
							for (const selector of splitSelectors(rule.selector)) {
								unhandledFallbacks.set(`${selector}\n${declaration.prop}`, declaration.value)
							}
							continue
						}

						for (const selector of splitSelectors(rule.selector)) {
							fallbackColors.set(`${selector}\n${declaration.prop}`, rgba)
						}
					}
				})
			})

			root.walkRules((rule) => {
				if (isInsideColorMixSupports(rule) || rule.nodes.length !== 1) {
					return
				}

				const declaration = rule.nodes[0]
				if (!declaration.value.startsWith('var(--')) {
					return
				}

				const selectors = splitSelectors(rule.selector)
				const normalSelectors = []
				const fallbackSelectorsByColor = new Map()

				for (const selector of selectors) {
					const color = fallbackColors.get(`${selector}\n${declaration.prop}`)
					if (!color) {
						const unhandled = unhandledFallbacks.get(`${selector}\n${declaration.prop}`)
						if (unhandled) {
							declaration.warn(result, `could not flatten translucent color fallback for ${selector} ${declaration.prop}: ${unhandled}`)
						}
						normalSelectors.push(selector)
						continue
					}

					replaced.add(`${selector}\n${declaration.prop}`)

					const fallbackSelectors = fallbackSelectorsByColor.get(color) ?? []
					fallbackSelectors.push(selector)
					fallbackSelectorsByColor.set(color, fallbackSelectors)
				}

				if (fallbackSelectorsByColor.size === 0) {
					return
				}

				const rules = []
				if (normalSelectors.length > 0) {
					const normalRule = rule.clone()
					normalRule.selector = normalSelectors.join(',')
					rules.push(normalRule)
				}

				for (const [color, fallbackSelectors] of fallbackSelectorsByColor) {
					const fallbackRule = rule.clone()
					fallbackRule.selector = fallbackSelectors.join(',')
					fallbackRule.nodes[0].value = color
					rules.push(fallbackRule)
				}

				rule.replaceWith(...rules)
			})

			for (const key of fallbackColors.keys()) {
				if (!replaced.has(key)) {
					result.warn(`could not flatten translucent color fallback for ${key.replace('\n', ' ')}`)
				}
			}
		},
	}
}

flattenTranslucentColorMixFallbacks.postcss = true

function flattenCascadeLayers() {
	return {
		postcssPlugin: 'flatten-cascade-layers',
		AtRule(atRule) {
			if (atRule.name.toLowerCase() !== 'layer') {
				return
			}

			if (atRule.nodes?.length) {
				atRule.replaceWith(...atRule.nodes)
				return
			}

			atRule.remove()
		},
	}
}

flattenCascadeLayers.postcss = true

function normalizeWherePseudoClasses() {
	return {
		postcssPlugin: 'normalize-where-pseudo-classes',
		Rule(rule) {
			if (rule.selector?.includes(':where(')) {
				rule.selector = rule.selector.replaceAll(':where(', ':is(')
			}
		},
	}
}

normalizeWherePseudoClasses.postcss = true

function splitSelectors(selector) {
	return selector.split(',').map(part => part.trim()).filter(Boolean)
}

function isInsideColorMixSupports(node) {
	let parent = node.parent
	while (parent) {
		if (parent.type === 'atrule' && parent.name === 'supports' && parent.params.includes('color-mix(')) {
			return true
		}
		parent = parent.parent
	}
	return false
}

function colorMixRgbToRgba(value) {
	const match = value.match(/^color-mix\(\s*in\s+oklab,\s*rgb\(\s*([+-]?(?:\d+|\d*\.\d+))\s*,\s*([+-]?(?:\d+|\d*\.\d+))\s*,\s*([+-]?(?:\d+|\d*\.\d+))\s*\)\s+([+-]?(?:\d+|\d*\.\d+))%,\s*transparent\s*\)$/)
	if (!match) {
		return null
	}

	const [, red, green, blue, percentage] = match.map(Number)
	return `rgba(${red},${green},${blue},${percentage / 100})`
}

export default {
	plugins: [
		// Tailwind v4 emits the source utilities and theme variables.
		tailwindcss({
			base: './src',
			optimize: false,
		}),
		// Expands Tailwind transform shortcuts into Chrome 86-compatible transforms.
		postcssTransformShortcut(),
		// Keeps empty custom-property fallbacks parseable after Tailwind output.
		fixEmptyCssVariableFallbacks(),
		// Inlines root color variables before color-mix fallback generation.
		resolveRootVariablesInColorMix(),
		// Adds legacy gradient fallbacks without explicit oklab color-space hints.
		fallbackGradientColorSpaces(),
		// Removes cascade layers so older browser fallback order is deterministic.
		flattenCascadeLayers(),
		// Replaces solid var() fallbacks for translucent color-mix utilities with rgba().
		flattenTranslucentColorMixFallbacks(),
		// Main Chrome >= 86 compatibility pass for modern CSS syntax.
		postcssPresetEnv({
			stage: 2,
			browsers: 'Chrome >= 86',
			preserve: false,
			enableClientSidePolyfills: true,
			features: {
				'cascade-layers': false,
				'color-mix': true,
				'color-mix-variadic-function-arguments': true,
				'has-pseudo-class': { preserve: false },
				'is-pseudo-class': true,
				'logical-properties-and-values': true,
				'media-query-ranges': true,
				'nesting-rules': true,
				'oklab-function': true,
			},
			logical: {
				inlineDirection: 'left-to-right',
				blockDirection: 'top-to-bottom',
			},
		}),
		// Converts :where() into a selector form handled by the final :is() transform.
		normalizeWherePseudoClasses(),
		// Lowers :is() selectors for Chrome 86 after local selector normalization.
		postcssIsPseudoClass({
			preserve: false,
		}),
	],
}
