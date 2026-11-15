import { describe, expect, it } from 'vitest';
import { dropPrefixedBackdropFilter } from './cssfix';

describe('dropPrefixedBackdropFilter', () => {
	it('removes the prefixed copy next to the standard property', () => {
		const css = `.top {\n\tbackdrop-filter: blur(28px) saturate(1.4);\n\t-webkit-backdrop-filter: blur(28px) saturate(1.4);\n\tbackground: red;\n}`;
		expect(dropPrefixedBackdropFilter(css)).toBe(`.top {\n\tbackdrop-filter: blur(28px) saturate(1.4);\n\t\n\tbackground: red;\n}`);
	});

	it('handles either order and a last declaration without a semicolon', () => {
		expect(dropPrefixedBackdropFilter('.a{-webkit-backdrop-filter:blur(2px);backdrop-filter:blur(2px)}')).toBe('.a{backdrop-filter:blur(2px)}');
		expect(dropPrefixedBackdropFilter('.a{backdrop-filter:blur(2px);-webkit-backdrop-filter:blur(2px)}')).toBe('.a{backdrop-filter:blur(2px);}');
	});

	it('keeps a prefixed property on its own, and other blocks', () => {
		const css = '.a{-webkit-backdrop-filter:blur(2px)} .b{backdrop-filter:none} @media (width < 9px){.c{color:red}}';
		expect(dropPrefixedBackdropFilter(css)).toBe(css);
	});
});
