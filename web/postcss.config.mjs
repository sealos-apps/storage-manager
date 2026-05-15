import postcssIsPseudoClass from '@csstools/postcss-is-pseudo-class'
import tailwindcss from '@tailwindcss/postcss'
import postcssPresetEnv from 'postcss-preset-env'
import valueParser from 'postcss-value-parser'

function fallbackTransformProperties() {
	return {
		postcssPlugin: 'fallback-transform-properties',
		Rule(rule) {
			const transformDeclarations = []

			rule.walkDecls((declaration) => {
				if (!['translate', 'scale', 'rotate'].includes(declaration.prop)) {
					return
				}

				transformDeclarations.push(declaration)
			})

			if (!transformDeclarations.length) {
				return
			}

			const transformValue = transformDeclarations
				.map((declaration) => {
					if (declaration.prop === 'rotate') {
						return `rotate(${declaration.value})`
					}

					if (declaration.prop === 'scale') {
						return `scale(${declaration.value})`
					}

					return `translate(${declaration.value})`
				})
				.join(' ')

			transformDeclarations[transformDeclarations.length - 1].after({
				prop: 'transform',
				value: transformValue,
			})
		},
	}
}

fallbackTransformProperties.postcss = true

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

export default {
	plugins: [
		tailwindcss({
			base: './src',
			optimize: false,
		}),
		fallbackTransformProperties(),
		fixEmptyCssVariableFallbacks(),
		resolveRootVariablesInColorMix(),
		fallbackGradientColorSpaces(),
		flattenCascadeLayers(),
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
		normalizeWherePseudoClasses(),
		postcssIsPseudoClass({
			preserve: false,
		}),
	],
}
