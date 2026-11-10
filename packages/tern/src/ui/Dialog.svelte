<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		/** Whether the dialog is open; bind it. */
		open?: boolean;
		/** The heading, which labels the dialog. */
		title: string;
		/** A sentence under the title, read when the dialog opens. */
		description?: string;
		/** Close when the backdrop is clicked. Escape always closes. */
		dismissable?: boolean;
		size?: 'sm' | 'md' | 'lg';
		class?: string;
		children?: Snippet;
		/** Buttons at the bottom, such as Cancel and Save. */
		footer?: Snippet;
		onclose?: () => void;
	};
	let { open = $bindable(false), title, description, dismissable = true, size = 'md', class: klass = '', children, footer, onclose }: Props = $props();

	const uid = $props.id();
	let dialog = $state<HTMLDialogElement>();

	// The native dialog traps focus, makes the page inert, closes on Escape
	// and returns focus to what opened it.
	$effect(() => {
		if (!dialog) return;
		if (open && !dialog.open) dialog.showModal();
		else if (!open && dialog.open) dialog.close();
	});

	// Tab cycles inside the dialog instead of leaving for the browser's UI.
	function onkeydown(e: KeyboardEvent) {
		if (e.key !== 'Tab' || !dialog) return;
		const focusable = Array.from(
			dialog.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])')
		).filter((el) => el.offsetParent !== null || el === document.activeElement);
		if (focusable.length === 0) return;
		const first = focusable[0]!;
		const last = focusable[focusable.length - 1]!;
		if (e.shiftKey && document.activeElement === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && document.activeElement === last) {
			e.preventDefault();
			first.focus();
		}
	}
</script>

<dialog
	bind:this={dialog}
	class="tern-dialog {size} {klass}"
	aria-labelledby="tern-dialog-{uid}-title"
	aria-describedby={description ? `tern-dialog-${uid}-description` : undefined}
	onclose={() => {
		open = false;
		onclose?.();
	}}
	onclick={(e) => {
		if (dismissable && e.target === dialog) open = false;
	}}
	{onkeydown}
>
	<div class="panel">
		<header>
			<h2 id="tern-dialog-{uid}-title">{title}</h2>
			<button type="button" class="close" aria-label="Close" onclick={() => (open = false)}>
				<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" /></svg>
			</button>
		</header>
		{#if description}<p class="description" id="tern-dialog-{uid}-description">{description}</p>{/if}
		{#if children}<div class="body">{@render children()}</div>{/if}
		{#if footer}<footer>{@render footer()}</footer>{/if}
	</div>
</dialog>

<style>
	.tern-dialog {
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--tern-fg);
		max-width: calc(100vw - 2rem);
		width: 28rem;
		overflow: visible;
	}
	.sm {
		width: 22rem;
	}
	.lg {
		width: 40rem;
	}
	.tern-dialog::backdrop {
		background: oklch(0.2 0.02 240 / 0.45);
		backdrop-filter: blur(2px);
	}
	.tern-dialog[open] .panel {
		animation: rise var(--tern-duration) ease-out;
	}
	.panel {
		display: grid;
		gap: 1rem;
		padding: 1.25rem 1.5rem 1.5rem;
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: calc(var(--tern-radius) + 4px);
		box-shadow: var(--tern-shadow-lg);
	}
	header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}
	h2 {
		margin: 0;
		font-size: 1.0625rem;
		font-weight: 650;
		letter-spacing: -0.01em;
	}
	.description {
		margin: -0.5rem 0 0;
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
	.close {
		display: grid;
		place-items: center;
		width: 1.75rem;
		height: 1.75rem;
		margin: -0.25rem -0.5rem 0 0;
		border: 0;
		border-radius: var(--tern-radius-sm);
		background: transparent;
		color: var(--tern-muted);
		cursor: pointer;
	}
	.close:hover {
		background: var(--tern-surface-2);
		color: var(--tern-fg);
	}
	.close:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 1px;
	}
	footer {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 0.25rem;
	}
	@keyframes rise {
		from {
			opacity: 0;
			transform: translateY(6px) scale(0.985);
		}
	}
</style>
