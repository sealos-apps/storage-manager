import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import process from 'node:process'

const distDir = new URL('../dist/assets/', import.meta.url)

const cssFiles = readdirSync(distDir).filter(file => file.endsWith('.css'))
const unsupportedPatterns = [
	/@layer\b/,
	/oklch\(/,
	/oklab\(/,
	/:has\(/,
	/:is\(/,
	/:where\(/,
	/:not\(#\\#\)/,
	/var\([^)]*,\)/,
]

let failed = false

for (const file of cssFiles) {
	const css = readFileSync(join(distDir.pathname, file), 'utf8')

	for (const pattern of unsupportedPatterns) {
		if (pattern.test(css)) {
			failed = true
			console.error(`${file} still contains ${pattern}`)
		}
	}

	if (/(?:^|[;{])(?:translate|scale|rotate):/.test(css) && !/\btransform:/.test(css)) {
		failed = true
		console.error(`${file} contains independent transform properties without transform fallback`)
	}

	if (css.includes('box-shadow:var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)')
		&& !css.includes('--tw-inset-shadow:0 0 #0000;--tw-inset-ring-shadow:0 0 #0000;--tw-ring-offset-width:0px;--tw-ring-offset-color:#fff;--tw-ring-offset-shadow:0 0 #0000;--tw-ring-shadow:0 0 #0000;--tw-shadow:0 0 #0000')) {
		failed = true
		console.error(`${file} contains Tailwind box-shadow variables without Chrome 86 defaults`)
	}
}

if (failed) {
	process.exit(1)
}

console.log(`Checked ${cssFiles.length} CSS file(s) for Chrome 86 fallbacks.`)
