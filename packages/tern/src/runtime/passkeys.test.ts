import { describe, expect, it } from 'vitest';
import { fromBase64url, toBase64url } from './passkeys.js';

describe('base64url', () => {
	it('round-trips every length', () => {
		for (let n = 0; n < 40; n++) {
			const b = new Uint8Array(n).map((_, i) => (i * 37 + n) & 0xff);
			const s = toBase64url(b);
			expect(s).not.toMatch(/[+/=]/);
			expect(Array.from(fromBase64url(s))).toEqual(Array.from(b));
		}
	});
	it('reads what Go writes', () => {
		expect(new TextDecoder().decode(fromBase64url('MTIz'))).toBe('123');
		expect(toBase64url(new Uint8Array([0xfb, 0xff]))).toBe('-_8');
	});
});
