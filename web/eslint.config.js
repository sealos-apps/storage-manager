import antfu from '@antfu/eslint-config'

export default antfu({
	formatters: true,
	react: true,
	ignores: [
		'dist/**',
		'src/services/encore/client.ts',
	],
	stylistic: {
		indent: 'tab',
	},
})
