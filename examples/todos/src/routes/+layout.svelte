<script lang="ts">
	import { navigating } from '@ternkit/tern';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
</script>

{#if navigating.current}<div class="progress" aria-hidden="true"></div>{/if}
{@render children()}

<style>
	:global(*, *::before, *::after) {
		box-sizing: border-box;
	}
	:global(body) {
		margin: 0;
		font: 16px/1.6 system-ui, sans-serif;
		color: #1c1917;
		background: #f5f5f4;
	}
	:global(main) {
		max-width: 36rem;
		margin: 2.5rem auto;
		padding: 0 1.25rem;
	}
	:global(input, button) {
		font: inherit;
		padding: 0.45rem 0.8rem;
		border: 1px solid #d6d3d1;
		border-radius: 6px;
	}
	:global(button) {
		background: #0f766e;
		border-color: #0f766e;
		color: white;
		cursor: pointer;
	}
	:global(a) {
		color: #0f766e;
	}
	:global(.markdown pre) {
		background: #1c1917;
		color: #f5f5f4;
		padding: 0.8rem 1rem;
		border-radius: 6px;
		overflow-x: auto;
	}
	:global(.posts) {
		list-style: none;
		padding: 0;
	}
	:global(.posts small) {
		color: #78716c;
		margin-left: 0.5rem;
	}
	:global(.toc) {
		display: flex;
		gap: 1rem;
		font-size: 0.9rem;
	}
	:global(.error) {
		color: #b91c1c;
	}
	.progress {
		position: fixed;
		inset: 0 0 auto;
		height: 3px;
		background: #0f766e;
		animation: grow 1s ease-out;
	}
	@keyframes grow {
		from {
			transform: scaleX(0);
			transform-origin: left;
		}
	}
</style>
