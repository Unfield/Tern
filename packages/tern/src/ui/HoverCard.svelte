<script lang="ts">
	import type { Snippet } from 'svelte';
	import { float, type Align, type Side } from './floating.js';

	export type HoverCardTrigger = {
		'aria-describedby'?: string;
		onpointerenter: () => void;
		onpointerleave: () => void;
		onfocus: () => void;
		onblur: () => void;
		onkeydown: (e: KeyboardEvent) => void;
	};

	type Props = {
		/** The link or button it previews. Spread the props onto it. */
		trigger: Snippet<[HoverCardTrigger]>;
		/** The preview. Pointer and keyboard users see it; it is also linked as the trigger's description, so keep it short or make sure what it shows is reachable elsewhere. */
		children: Snippet;
		side?: Side;
		align?: Align;
		/** Milliseconds before it opens. */
		openDelay?: number;
		/** Milliseconds before it closes once the pointer leaves. */
		closeDelay?: number;
		class?: string;
	};
	let { trigger, children, side = 'bottom', align = 'center', openDelay = 500, closeDelay = 200, class: klass = '' }: Props = $props();

	const uid = $props.id();
	const id = `tern-hovercard-${uid}`;
	let open = $state(false);
	let card = $state<HTMLDivElement>();
	let anchorEl: Element | null = null;
	let timer: ReturnType<typeof setTimeout> | undefined;

	function later(fn: () => void, ms: number) {
		clearTimeout(timer);
		timer = setTimeout(fn, ms);
	}
	const show = (now = false) => {
		anchorEl = document.activeElement?.matches(`[aria-describedby~="${id}"]`) ? document.activeElement : (document.querySelector(`[aria-describedby~="${id}"]`) ?? anchorEl);
		if (now) {
			clearTimeout(timer);
			open = true;
		} else later(() => (open = true), openDelay);
	};
	const hide = (now = false) => (now ? ((open = false), clearTimeout(timer)) : later(() => (open = false), closeDelay));

	$effect(() => {
		if (!open || !card || !anchorEl) return;
		card.showPopover?.();
		return float(card, anchorEl, { side, align, offset: 8 });
	});
</script>

{@render trigger({
	'aria-describedby': id,
	onpointerenter: () => show(),
	onpointerleave: () => hide(),
	onfocus: () => show(true),
	onblur: () => hide(true),
	onkeydown: (e) => e.key === 'Escape' && hide(true)
})}
<div
	bind:this={card}
	{id}
	class="tern-hovercard {klass}"
	class:open
	popover="manual"
	role="none"
	onpointerenter={() => clearTimeout(timer)}
	onpointerleave={() => hide()}
>
	{#if open}{@render children()}{/if}
</div>

<style>
	.tern-hovercard {
		width: max-content;
		max-width: min(20rem, calc(100vw - 1rem));
		padding: 1rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		box-shadow: var(--tern-shadow-lg);
		font-size: 0.875rem;
	}
	.tern-hovercard:not(.open) {
		display: none;
	}
	.open {
		animation: tern-hover-in var(--tern-duration) ease-out;
	}
	@keyframes tern-hover-in {
		from {
			opacity: 0;
			transform: translateY(2px);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.open {
			animation: none;
		}
	}
</style>
