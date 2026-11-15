import type { Plugin } from 'vite';

/**
 * Lightning CSS, which minifies CSS in Vite's builds, reads
 * `-webkit-backdrop-filter` after `backdrop-filter` as overriding it, and
 * drops the standard property: Chrome and Firefox then get no blur at all,
 * and a release build looks different from `tern dev`, which doesn't
 * minify. The prefixed copy is removed before minifying instead, and
 * Lightning CSS adds it back for the Safari versions that need it.
 */
export function backdropFilterPlugin(): Plugin {
	return {
		name: 'tern:backdrop-filter',
		apply: 'build',
		enforce: 'pre',
		transform(code, id) {
			if (!isCSS(id) || !code.includes('-webkit-backdrop-filter')) return null;
			const fixed = dropPrefixedBackdropFilter(code);
			return fixed === code ? null : { code: fixed, map: null };
		}
	};
}

function isCSS(id: string): boolean {
	const path = id.split('?')[0] ?? id;
	return /\.(css|pcss|postcss)$/.test(path) || /[?&]lang\.css$/.test(id) || /[?&]type=style\b/.test(id);
}

const block = /\{[^{}]*\}/g;
const standard = /(^|[{;\s])backdrop-filter\s*:/;
const prefixed = /(^|[{;\s])-webkit-backdrop-filter\s*:[^;}]*(;|(?=\}))/g;

/**
 * dropPrefixedBackdropFilter removes `-webkit-backdrop-filter` from each
 * declaration block that also sets `backdrop-filter`. Blocks with only the
 * prefixed property keep it.
 */
export function dropPrefixedBackdropFilter(css: string): string {
	return css.replace(block, (b) => (standard.test(b) ? b.replace(prefixed, '$1') : b));
}
