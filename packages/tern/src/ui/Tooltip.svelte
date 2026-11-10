<script lang="ts">
	import type { Snippet } from 'svelte';
	import { float, type Side } from './floating.js';

	type Props = {
		/** The tooltip's text. It describes the trigger; don't put anything interactive in it. */
		text: string;
		side?: Side;
		/** How long to wait before showing, in milliseconds. */
		delay?: number;
		/** The trigger, which must be focusable, such as a Button. It receives aria-describedby through the snippet's props. */
		children: Snippet<[{ 'aria-describedby': string }]>;
	};
	let { text, side = 'top', delay = 300, children }: Props = $props();
	const uid = $props.id();
	let open = $state(false);
	let wrap = $state<HTMLSpanElement>();
	let tip = $state<HTMLSpanElement>();
	let timer: ReturnType<typeof setTimeout> | undefined;

	const show = (now = false) => {
		clearTimeout(timer);
		if (now) open = true;
		else timer = setTimeout(() => (open = true), delay);
	};
	const hide = () => {
		clearTimeout(timer);
		open = false;
	};

	$effect(() => {
		if (!open || !tip || !wrap) return;
		tip.showPopover?.();
		const stop = float(tip, wrap.firstElementChild ?? wrap, { side, align: 'center', offset: 6 });
		return () => {
			stop();
			tip?.hidePopover?.();
		};
	});
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<span
	bind:this={wrap}
	class="tern-tooltip"
	onpointerenter={() => show()}
	onpointerleave={hide}
	onfocusin={() => show(true)}
	onfocusout={hide}
	onkeydown={(e) => e.key === 'Escape' && hide()}
>
	{@render children({ 'aria-describedby': `tern-tooltip-${uid}` })}
	<span bind:this={tip} role="tooltip" id="tern-tooltip-{uid}" class="tip" class:open popover="manual">{text}</span>
</span>

<style>
	.tern-tooltip {
		display: inline-flex;
	}
	.tip {
		width: max-content;
		max-width: 16rem;
		padding: 0.35rem 0.55rem;
		border: 0;
		border-radius: var(--tern-radius-sm);
		background: var(--tern-fg);
		color: var(--tern-bg);
		font-size: 0.75rem;
		line-height: 1.35;
		pointer-events: none;
	}
	.tip.open {
		animation: tern-tip-in var(--tern-duration) ease-out;
	}
	@keyframes tern-tip-in {
		from {
			opacity: 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.tip.open {
			animation: none;
		}
	}
</style>
