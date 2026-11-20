<!--
	A bar across the top of the page: the app's name, its main links, and
	actions such as a sign-in button. It stays at the top while the page
	scrolls, and every size in it is a whole pixel: a bar on fractional
	pixels twitches while the page scrolls, because browsers round it to the
	screen differently from frame to frame.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		brand,
		children,
		actions,
		label = 'Main',
		translucent = false,
		class: klass = ''
	}: {
		/** The app's name or logo, usually a link home. */
		brand?: Snippet;
		/** The main links: plain <a> elements; the current page's gets aria-current="page". */
		children?: Snippet;
		/** On the right: a sign-in button, a menu, a theme switch. */
		actions?: Snippet;
		/** The navigation's accessible name. */
		label?: string;
		/** A see-through, blurred background. The bar is then fixed rather than sticky, with a spacer under it: blur behind a sticky bar twitches in Safari. */
		translucent?: boolean;
		class?: string;
	} = $props();
</script>

<header class="tern-header {klass}" class:translucent>
	<div class="row">
		{#if brand}<div class="brand">{@render brand()}</div>{/if}
		{#if children}<nav aria-label={label}>{@render children()}</nav>{/if}
		{#if actions}<div class="actions">{@render actions()}</div>{/if}
	</div>
</header>
{#if translucent}<div class="tern-header-spacer" aria-hidden="true"></div>{/if}

<style>
	/* Whole pixels throughout: 56px of content and a 1px border. */
	.tern-header {
		position: sticky;
		top: 0;
		z-index: 30;
		height: 56px;
		border-bottom: 1px solid var(--tern-border);
		background: var(--tern-bg);
		color: var(--tern-fg);
		font-size: 14px;
		line-height: 20px;
	}
	.tern-header.translucent {
		position: fixed;
		left: 0;
		right: 0;
		background: color-mix(in oklch, var(--tern-bg) 85%, transparent);
		-webkit-backdrop-filter: saturate(1.5) blur(12px);
		backdrop-filter: saturate(1.5) blur(12px);
	}
	.tern-header-spacer {
		height: 57px;
	}
	.row {
		display: flex;
		align-items: center;
		gap: 16px;
		box-sizing: border-box;
		height: 56px;
		max-width: var(--tern-header-width, none);
		margin: 0 auto;
		padding: 0 24px;
	}
	.brand {
		display: flex;
		align-items: center;
		flex: none;
		gap: 8px;
		font-size: 16px;
		line-height: 24px;
		font-weight: 650;
	}
	.brand :global(a) {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		color: inherit;
		text-decoration: none;
	}
	nav {
		display: flex;
		align-items: center;
		gap: 4px;
		min-width: 0;
		overflow-x: auto;
		scrollbar-width: none;
	}
	nav::-webkit-scrollbar {
		display: none;
	}
	nav :global(a) {
		flex: none;
		padding: 6px 12px;
		border-radius: var(--tern-radius-sm);
		color: var(--tern-muted);
		text-decoration: none;
		white-space: nowrap;
		transition:
			color var(--tern-duration),
			background-color var(--tern-duration);
	}
	nav :global(a:hover) {
		color: var(--tern-fg);
		background: var(--tern-surface-2);
	}
	nav :global(a[aria-current='page']) {
		color: var(--tern-fg);
		font-weight: 550;
	}
	nav :global(a:focus-visible) {
		outline: 2px solid var(--tern-ring);
		outline-offset: -2px;
	}
	.actions {
		display: flex;
		align-items: center;
		flex: none;
		gap: 8px;
		margin-left: auto;
	}
	@media (max-width: 40rem) {
		.row {
			gap: 12px;
			padding: 0 16px;
		}
		nav :global(a) {
			padding: 6px 8px;
		}
	}
</style>
