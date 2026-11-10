<script lang="ts">
	import type { Snippet } from 'svelte';
	import MenuList from './MenuList.svelte';
	import type { MenuItem } from './menu.js';

	type Props = {
		items: MenuItem[];
		/** Names the menu for screen readers, such as "File actions". */
		label: string;
		/** The area that has the menu. Right-click, a long press, Shift+F10 or the menu key opens it. */
		children: Snippet;
		class?: string;
	};
	let { items, label, children, class: klass = '' }: Props = $props();

	const uid = $props.id();
	let at = $state<{ x: number; y: number } | null>(null);
	let returnTo: HTMLElement | null = null;
	let press: { timer: ReturnType<typeof setTimeout>; x: number; y: number } | null = null;

	function openAt(x: number, y: number) {
		returnTo = document.activeElement as HTMLElement | null;
		at = { x, y };
	}

	function cancelPress() {
		if (press) clearTimeout(press.timer);
		press = null;
	}
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="tern-context {klass}"
	oncontextmenu={(e) => {
		e.preventDefault();
		cancelPress();
		openAt(e.clientX, e.clientY);
	}}
	onkeydown={(e) => {
		if ((e.key === 'F10' && e.shiftKey) || e.key === 'ContextMenu') {
			e.preventDefault();
			const r = (e.target as HTMLElement).getBoundingClientRect();
			openAt(r.left + 8, r.bottom);
		}
	}}
	onpointerdown={(e) => {
		// iOS doesn't fire contextmenu on a long press.
		if (e.pointerType !== 'touch') return;
		const { clientX: x, clientY: y } = e;
		press = { x, y, timer: setTimeout(() => openAt(x, y), 550) };
	}}
	onpointermove={(e) => {
		if (press && Math.hypot(e.clientX - press.x, e.clientY - press.y) > 8) cancelPress();
	}}
	onpointerup={cancelPress}
	onpointercancel={cancelPress}
>
	{@render children()}
</div>
{#if at}
	<MenuList
		{items}
		id="tern-context-{uid}"
		{label}
		anchor={at}
		onclose={(how) => {
			at = null;
			if (how.returnFocus) returnTo?.focus();
		}}
	/>
{/if}

<style>
	.tern-context {
		-webkit-touch-callout: none;
	}
</style>
