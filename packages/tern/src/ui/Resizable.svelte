<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import type { Snippet } from 'svelte';

	type Props = {
		/** The first pane's share, in percent; bind it. */
		size?: number;
		min?: number;
		max?: number;
		direction?: 'horizontal' | 'vertical';
		/** Names the handle, such as "Resize sidebar". */
		label?: string;
		first: Snippet;
		second: Snippet;
		class?: string;
	};
	let { size = $bindable(50), min = 15, max = 85, direction = 'horizontal', label: labelProp, first, second, class: klass = '' }: Props = $props();
	const label = $derived(labelProp ?? tb('tern.ui.resizable.label'));

	const uid = $props.id();
	let root = $state<HTMLDivElement>();
	let dragging = $state(false);
	const clamp = (v: number) => Math.round(Math.min(max, Math.max(min, v)) * 10) / 10;

	function move(e: PointerEvent) {
		if (!dragging || !root) return;
		const r = root.getBoundingClientRect();
		size = clamp(direction === 'horizontal' ? ((e.clientX - r.left) / r.width) * 100 : ((e.clientY - r.top) / r.height) * 100);
	}
	function onkeydown(e: KeyboardEvent) {
		const back = direction === 'horizontal' ? 'ArrowLeft' : 'ArrowUp';
		const fwd = direction === 'horizontal' ? 'ArrowRight' : 'ArrowDown';
		const moves: Record<string, number> = { [back]: size - 5, [fwd]: size + 5, Home: min, End: max };
		if (!(e.key in moves)) return;
		e.preventDefault();
		size = clamp(moves[e.key]!);
	}
</script>

<div bind:this={root} class="tern-resizable {direction} {klass}" class:dragging>
	<div class="pane" id="tern-resizable-{uid}-first" style:flex-basis="{size}%">{@render first()}</div>
	<!-- A focusable window splitter, as the ARIA pattern has it. -->
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div
		class="handle"
		role="separator"
		tabindex="0"
		aria-label={label}
		aria-controls="tern-resizable-{uid}-first"
		aria-orientation={direction === 'horizontal' ? 'vertical' : 'horizontal'}
		aria-valuenow={size}
		aria-valuemin={min}
		aria-valuemax={max}
		onpointerdown={(e) => {
			dragging = true;
			(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		}}
		onpointermove={move}
		onpointerup={() => (dragging = false)}
		onpointercancel={() => (dragging = false)}
		{onkeydown}
	></div>
	<div class="pane second">{@render second()}</div>
</div>

<style>
	.tern-resizable {
		display: flex;
		width: 100%;
		height: 100%;
		min-height: 0;
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		overflow: hidden;
	}
	.vertical {
		flex-direction: column;
	}
	.pane {
		flex-shrink: 0;
		min-width: 0;
		min-height: 0;
		overflow: auto;
	}
	.second {
		flex: 1;
	}
	.handle {
		position: relative;
		flex: none;
		background: var(--tern-border);
		touch-action: none;
		outline: none;
	}
	.horizontal > .handle {
		width: 1px;
		cursor: col-resize;
	}
	.vertical > .handle {
		height: 1px;
		cursor: row-resize;
	}
	/* A wider target than the line. */
	.handle::before {
		content: '';
		position: absolute;
		inset: -4px;
	}
	.handle:hover,
	.handle:focus-visible,
	.dragging > .handle {
		background: var(--tern-accent);
	}
	.handle:focus-visible::after {
		content: '';
		position: absolute;
		inset: -2px;
		border-radius: 2px;
		box-shadow: 0 0 0 2px var(--tern-ring);
	}
	.dragging {
		user-select: none;
	}
</style>
