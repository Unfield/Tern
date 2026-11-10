<script lang="ts">
	import type { Snippet } from 'svelte';
	import { float, focusables, type Align, type Side } from './floating.js';

	export type PopoverTrigger = {
		id: string;
		'aria-haspopup': 'dialog';
		'aria-expanded': boolean;
		'aria-controls': string;
		onclick: () => void;
	};

	type Props = {
		/** Whether it's open; bind it. */
		open?: boolean;
		/** A heading shown at the top, which labels the popover. */
		title?: string;
		/** Labels the popover when it has no title. */
		label?: string;
		side?: Side;
		align?: Align;
		/** The button that opens it. Spread the props onto it: {#snippet trigger(props)}<Button {...props}>…</Button>{/snippet} */
		trigger: Snippet<[PopoverTrigger]>;
		children: Snippet;
		class?: string;
		/** The trigger's id, when it needs a particular one, such as a Field's. */
		triggerId?: string;
		onclose?: () => void;
	};
	let { open = $bindable(false), title, label, side = 'bottom', align = 'center', trigger, children, class: klass = '', triggerId, onclose }: Props = $props();

	const uid = $props.id();
	const id = `tern-popover-${uid}`;
	let panel = $state<HTMLDivElement>();
	const tid = $derived(triggerId ?? `${id}-trigger`);
	const anchor = () => document.getElementById(tid);

	function close(returnFocus: boolean) {
		if (!open) return;
		open = false;
		onclose?.();
		if (returnFocus) anchor()?.focus();
	}

	$effect(() => {
		if (!open || !panel) return;
		const a = anchor();
		if (!a) return;
		panel.showPopover?.();
		const stop = float(panel, a, { side, align, offset: 8 });
		(focusables(panel)[0] ?? panel).focus({ preventScroll: true });
		const away = (e: PointerEvent) => {
			const t = e.target as Node;
			if (!panel?.contains(t) && !a.contains(t)) close(false);
		};
		document.addEventListener('pointerdown', away, true);
		return () => {
			stop();
			document.removeEventListener('pointerdown', away, true);
		};
	});
</script>

{@render trigger({
	id: tid,
	'aria-haspopup': 'dialog',
	'aria-expanded': open,
	'aria-controls': id,
	onclick: () => (open ? close(false) : (open = true))
})}
{#if open}
	<div
		bind:this={panel}
		{id}
		class="tern-popover {klass}"
		role="dialog"
		aria-labelledby={title ? `${id}-title` : undefined}
		aria-label={title ? undefined : label}
		tabindex="-1"
		popover="manual"
		onkeydown={(e) => {
			if (e.key === 'Escape') {
				e.stopPropagation();
				close(true);
			}
		}}
		onfocusout={(e) => {
			const to = e.relatedTarget as Node | null;
			if (to && !panel?.contains(to) && !anchor()?.contains(to)) close(false);
		}}
	>
		{#if title}<h2 class="title" id="{id}-title">{title}</h2>{/if}
		{@render children()}
	</div>
{/if}

<style>
	.tern-popover {
		width: max-content;
		max-width: min(22rem, calc(100vw - 1rem));
		padding: 1rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		box-shadow: var(--tern-shadow-lg);
		font-size: 0.875rem;
		outline: none;
		animation: tern-pop-in var(--tern-duration) ease-out;
	}
	.title {
		margin: 0 0 0.5rem;
		font-size: 0.9375rem;
		font-weight: 600;
	}
	@keyframes tern-pop-in {
		from {
			opacity: 0;
			transform: scale(0.97);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.tern-popover {
			animation: none;
		}
	}
</style>
