import { describe, expect, it } from 'vitest'

import componentsConfig from '../../components.json' with { type: 'json' }

describe('shadcn configuration', () => {
	it('targets repository source directories for official CLI component output', () => {
		expect(componentsConfig.aliases).toMatchObject({
			components: 'src/components',
			hooks: 'src/hooks',
			lib: 'src/utils',
			ui: 'src/components/ui',
			utils: 'src/utils',
		})
	})
})
