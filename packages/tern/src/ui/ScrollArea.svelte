<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		/** Names the region, so keyboard and screen reader users know what scrolls. */
		label: string;
		/** The most it may grow before scrolling, such as "20rem". */
		maxHeight?: string;
		orientation?: 'vertical' | 'horizontal' | 'both';
		children: Snippet;
		class?: string;
	};
	let { label, maxHeight = '20rem', orientation = 'vertical', children, class: klass = '' }: Props = $props();
</script>

<!-- Focusable, so keyboards can scroll it, with thin scrollbars in the theme's colors. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div class="tern-scroll {orientation} {klass}" role="region" aria-label={label} tabindex="0" style:max-height={orientation === 'horizontal' ? undefined : maxHeight}>
	{@render children()}
</div>

<style>
	.tern-scroll {
		scrollbar-width: thin;
		scrollbar-color: var(--tern-border-strong) transparent;
		border-radius: var(--tern-radius-sm);
	}
	.vertical {
		overflow-y: auto;
		overflow-x: hidden;
	}
	.horizontal {
		overflow-x: auto;
		overflow-y: hidden;
	}
	.both {
		overflow: auto;
	}
	.tern-scroll:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
	}
</style>
