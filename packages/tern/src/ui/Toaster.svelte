<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import { dismiss, toasts, type Toast } from './toast.svelte.js';

	type Props = { position?: 'bottom-right' | 'bottom-left' | 'top-right' | 'top-left' };
	let { position = 'bottom-right' }: Props = $props();

	// Each toast's timer pauses while it is hovered or focused.
	function timed(node: HTMLElement, t: Toast) {
		if (!t.duration) return;
		let left = t.duration;
		let started = Date.now();
		let timer = setTimeout(() => dismiss(t.id), left);
		const pause = () => {
			clearTimeout(timer);
			left -= Date.now() - started;
		};
		const resume = () => {
			started = Date.now();
			clearTimeout(timer);
			timer = setTimeout(() => dismiss(t.id), Math.max(left, 1000));
		};
		node.addEventListener('pointerenter', pause);
		node.addEventListener('pointerleave', resume);
		node.addEventListener('focusin', pause);
		node.addEventListener('focusout', resume);
		return { destroy: () => clearTimeout(timer) };
	}
	const polite = $derived(toasts.filter((t) => t.tone !== 'danger'));
	const urgent = $derived(toasts.filter((t) => t.tone === 'danger'));
</script>

{#snippet list(items: Toast[])}
	{#each items as t (t.id)}
		<li class="toast {t.tone}" use:timed={t}>
			<div class="text">
				<p class="message">{t.message}</p>
				{#if t.description}<p class="description">{t.description}</p>{/if}
			</div>
			<button type="button" aria-label={tb('tern.ui.toaster.dismiss')} onclick={() => dismiss(t.id)}>
				<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" /></svg>
			</button>
		</li>
	{/each}
{/snippet}

<section class="tern-toaster {position}" aria-label={tb('tern.ui.toaster.label')}>
	<!-- Errors are announced at once; the rest when the reader is idle. -->
	<ol aria-live="assertive" aria-relevant="additions">{@render list(urgent)}</ol>
	<ol aria-live="polite" aria-relevant="additions">{@render list(polite)}</ol>
</section>

<style>
	.tern-toaster {
		position: fixed;
		z-index: 100;
		display: grid;
		gap: 0.5rem;
		width: min(22rem, calc(100vw - 2rem));
		pointer-events: none;
	}
	.bottom-right {
		right: 1rem;
		bottom: 1rem;
	}
	.bottom-left {
		left: 1rem;
		bottom: 1rem;
	}
	.top-right {
		right: 1rem;
		top: 1rem;
	}
	.top-left {
		left: 1rem;
		top: 1rem;
	}
	ol {
		display: grid;
		gap: 0.5rem;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.toast {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
		padding: 0.8rem 0.9rem;
		background: var(--tern-surface);
		color: var(--tern-fg);
		border: 1px solid var(--tern-border);
		border-left: 3px solid var(--tern-border-strong);
		border-radius: var(--tern-radius);
		box-shadow: var(--tern-shadow-lg);
		pointer-events: auto;
		animation: slide var(--tern-duration) ease-out;
	}
	.success {
		border-left-color: var(--tern-success);
	}
	.danger {
		border-left-color: var(--tern-danger);
	}
	.warning {
		border-left-color: var(--tern-warning);
	}
	.text {
		flex: 1;
		min-width: 0;
	}
	p {
		margin: 0;
	}
	.message {
		font-size: 0.875rem;
		font-weight: 550;
	}
	.description {
		margin-top: 0.15rem;
		font-size: 0.8125rem;
		color: var(--tern-muted);
	}
	button {
		display: grid;
		place-items: center;
		width: 1.5rem;
		height: 1.5rem;
		border: 0;
		border-radius: var(--tern-radius-sm);
		background: transparent;
		color: var(--tern-muted);
		cursor: pointer;
	}
	button:hover {
		background: var(--tern-surface-2);
	}
	button:focus-visible {
		outline: 2px solid var(--tern-ring);
	}
	@keyframes slide {
		from {
			opacity: 0;
			transform: translateY(6px);
		}
	}
</style>
