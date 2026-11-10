<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		/** The tooltip's text. It describes the trigger; don't put anything interactive in it. */
		text: string;
		side?: 'top' | 'bottom';
		/** How long to wait before showing, in milliseconds. */
		delay?: number;
		/** The trigger, which must be focusable, such as a Button. It receives aria-describedby through the snippet's props. */
		children: Snippet<[{ 'aria-describedby': string }]>;
	};
	let { text, side = 'top', delay = 300, children }: Props = $props();
	const uid = $props.id();
	let open = $state(false);
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
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<span
	class="tern-tooltip"
	onpointerenter={() => show()}
	onpointerleave={hide}
	onfocusin={() => show(true)}
	onfocusout={hide}
	onkeydown={(e) => e.key === 'Escape' && hide()}
>
	{@render children({ 'aria-describedby': `tern-tooltip-${uid}` })}
	<span role="tooltip" id="tern-tooltip-{uid}" class="tip {side}" class:open>{text}</span>
</span>

<style>
	.tern-tooltip {
		position: relative;
		display: inline-flex;
	}
	.tip {
		position: absolute;
		left: 50%;
		z-index: 60;
		width: max-content;
		max-width: 16rem;
		padding: 0.35rem 0.55rem;
		border-radius: var(--tern-radius-sm);
		background: var(--tern-fg);
		color: var(--tern-bg);
		font-size: 0.75rem;
		line-height: 1.35;
		pointer-events: none;
		opacity: 0;
		visibility: hidden;
		translate: -50% 0;
		transition:
			opacity var(--tern-duration),
			visibility var(--tern-duration);
	}
	.top {
		bottom: calc(100% + 0.4rem);
	}
	.bottom {
		top: calc(100% + 0.4rem);
	}
	.open {
		opacity: 1;
		visibility: visible;
	}
</style>
