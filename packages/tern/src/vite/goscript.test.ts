import { compile } from 'svelte/compiler';
import { describe, expect, it } from 'vitest';
import { exportedNames, liveBindings, wrapLiveMarkup } from './goscript.js';

describe('live components', () => {
	const source = `
import "github.com/Unfield/Tern/goscript"
var online = goscript.Presence("room")
func Setup(c *goscript.Component) {
	count := goscript.State[int](c, "count", 0)
	goscript.Derived(c, "doubled", func() int { return count.Get() * 2 })
	online.Show(c, "online")
	c.Func("add", func(n int) {})
}`;

	it('finds the names the markup can use', () => {
		expect(exportedNames(source)).toEqual({ values: ['count', 'doubled', 'online'], funcs: ['add'] });
	});

	it('finds the names rooms show, after their key', () => {
		const rooms = `room.Show(c, key, "people")\nboards.Show(c, "board:"+strconv.Itoa(id), "here")\nr.Show(c, fmt.Sprint("b", id), "crowd")`;
		expect(exportedNames(rooms).values).toEqual(['people', 'here', 'crowd']);
	});

	it('binds them, and live, unless the Go code uses those names', () => {
		const code = liveBindings('c1', ['count'], ['add']);
		expect(code).toContain('const live = __go;');
		expect(code).toContain(`const count = $derived(__go.state["count"] as any);`);
		expect(code).toContain(`const add: any = __go.fn("add");`);
		expect(liveBindings('c1', ['live'], [])).not.toContain('const live = __go;');
	});

	it('renders the markup once state has arrived, keeping special elements outside', () => {
		const input = `<script lang="go" live>\nfunc Setup() {}\n</script>\n\n<svelte:window onkeydown={add} />\n<p>{count}</p>\n<style>\n\tp { color: red }\n</style>\n`;
		const out = wrapLiveMarkup(input);
		expect(out).toMatch(/\{#if __go\.loaded\}[\s\S]*<p>\{count\}<\/p>[\s\S]*\{\/if\}/);
		const inside = out.slice(out.indexOf('{#if'), out.indexOf('{/if}'));
		expect(inside).not.toContain('<svelte:window');
		expect(inside).not.toContain('<style');
		// The markup stays on its line, for compile errors.
		expect(out.split('\n').findIndex((l) => l.includes('<p>{count}</p>'))).toBe(input.split('\n').findIndex((l) => l.includes('<p>{count}</p>')));
		expect(out).toContain('{@html __go.stateScript}');
		// The result is a valid component once the script is replaced.
		const ts = out.replace(/<script lang="go" live>[\s\S]*?<\/script>/, `<script lang="ts">${liveBindings('c1', ['count'], ['add'])}</script>`);
		expect(() => compile(ts.replace('lang="ts"', ''), { generate: 'client', filename: 'X.svelte' })).not.toThrow(/svelte:window|block/);
	});
});
