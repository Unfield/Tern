<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import type { Snippet } from 'svelte';
	import { trapTab } from './floating.js';

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
		/** center for a dialog; an edge slides it in from there, as Sheet and Drawer do. */
		side?: 'center' | 'left' | 'right' | 'top' | 'bottom';
		/** alertdialog for confirmations that interrupt, as AlertDialog uses. */
		role?: 'dialog' | 'alertdialog';
		/** Show the close button in the corner. */
		closeButton?: boolean;
		/** Drag the panel down to close it, with a handle at the top. For bottom sheets on touch screens. */
		draggable?: boolean;
		/** The element to focus when it opens, instead of the first focusable one. */
		initialFocus?: () => HTMLElement | null | undefined;
		class?: string;
		children?: Snippet;
		/** Buttons at the bottom, such as Cancel and Save. */
		footer?: Snippet;
		onclose?: () => void;
	};
	let {
		open = $bindable(false),
		title,
		description,
		dismissable = true,
		size = 'md',
		side = 'center',
		role = 'dialog',
		closeButton = true,
		draggable = false,
		initialFocus,
		class: klass = '',
		children,
		footer,
		onclose
	}: Props = $props();

	const uid = $props.id();
	let dialog = $state<HTMLDialogElement>();

	// The native dialog makes the page inert, closes on Escape and returns
	// focus to what opened it.
	$effect(() => {
		if (!dialog) return;
		if (open && !dialog.open) {
			dialog.showModal();
			const el = initialFocus?.();
			if (el) el.focus();
		} else if (!open && dialog.open) dialog.close();
	});

	// Dragging a bottom sheet down closes it once it passes a quarter of its height.
	let drag = $state(0);
	let dragStart: number | null = null;
	function dragDown(e: PointerEvent) {
		if (!draggable) return;
		dragStart = e.clientY;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}
	function dragMove(e: PointerEvent) {
		if (dragStart !== null) drag = Math.max(0, e.clientY - dragStart);
	}
	function dragEnd(e: PointerEvent) {
		if (dragStart === null) return;
		const panel = (e.currentTarget as HTMLElement).closest('.panel') as HTMLElement | null;
		if (panel && drag > panel.offsetHeight / 4) open = false;
		dragStart = null;
		drag = 0;
	}
</script>

<dialog
	bind:this={dialog}
	class="tern-dialog {size} {side} {klass}"
	{role}
	aria-labelledby="tern-dialog-{uid}-title"
	aria-describedby={description ? `tern-dialog-${uid}-description` : undefined}
	onclose={() => {
		open = false;
		onclose?.();
	}}
	onclick={(e) => {
		if (dismissable && e.target === dialog) open = false;
	}}
	onkeydown={(e) => dialog && trapTab(dialog, e)}
>
	<div class="panel" style:translate={drag ? `0 ${drag}px` : undefined}>
		{#if draggable}
			<div class="handle" aria-hidden="true" onpointerdown={dragDown} onpointermove={dragMove} onpointerup={dragEnd} onpointercancel={dragEnd}><span></span></div>
		{/if}
		<header>
			<h2 id="tern-dialog-{uid}-title">{title}</h2>
			{#if closeButton}
				<button type="button" class="close" aria-label={tb('tern.ui.dialog.close')} onclick={() => (open = false)}>
					<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" /></svg>
				</button>
			{/if}
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
	/* Sheets: full height or width, against an edge. */
	.tern-dialog:not(.center) {
		margin: 0;
		max-width: 100vw;
		max-height: 100vh;
	}
	.tern-dialog.left,
	.tern-dialog.right {
		height: 100vh;
		width: min(24rem, 100vw - 3rem);
	}
	.tern-dialog.left {
		margin-right: auto;
	}
	.tern-dialog.right {
		margin-left: auto;
	}
	.tern-dialog.top,
	.tern-dialog.bottom {
		width: 100vw;
	}
	.tern-dialog.top {
		margin-bottom: auto;
	}
	.tern-dialog.bottom {
		margin-top: auto;
	}
	.tern-dialog:not(.center) .panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		box-sizing: border-box;
		overflow: auto;
		border-radius: 0;
	}
	.tern-dialog.left footer,
	.tern-dialog.right footer {
		margin-top: auto;
	}
	.left .panel {
		border-width: 0 1px 0 0;
	}
	.right .panel {
		border-width: 0 0 0 1px;
	}
	.top .panel {
		border-width: 0 0 1px;
		border-radius: 0 0 calc(var(--tern-radius) + 4px) calc(var(--tern-radius) + 4px);
	}
	.bottom .panel {
		max-height: 85vh;
		border-width: 1px 0 0;
		border-radius: calc(var(--tern-radius) + 4px) calc(var(--tern-radius) + 4px) 0 0;
	}
	.tern-dialog.left[open] .panel {
		animation-name: from-left;
	}
	.tern-dialog.right[open] .panel {
		animation-name: from-right;
	}
	.tern-dialog.top[open] .panel {
		animation-name: from-top;
	}
	.tern-dialog.bottom[open] .panel {
		animation-name: from-bottom;
	}
	.handle {
		display: grid;
		place-items: center;
		margin: -0.75rem 0 -0.25rem;
		padding: 0.5rem;
		cursor: grab;
		touch-action: none;
	}
	.handle span {
		width: 2.5rem;
		height: 0.3rem;
		border-radius: 1rem;
		background: var(--tern-border-strong);
	}
	@keyframes from-left {
		from {
			transform: translateX(-100%);
		}
	}
	@keyframes from-right {
		from {
			transform: translateX(100%);
		}
	}
	@keyframes from-top {
		from {
			transform: translateY(-100%);
		}
	}
	@keyframes from-bottom {
		from {
			transform: translateY(100%);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.tern-dialog[open] .panel {
			animation: none !important;
		}
	}
	@keyframes rise {
		from {
			opacity: 0;
			transform: translateY(6px) scale(0.985);
		}
	}
</style>
