<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import Button from './Button.svelte';
	import Dialog from './Dialog.svelte';
	import type { Snippet } from 'svelte';

	type Props = {
		/** Whether it's open; bind it. */
		open?: boolean;
		/** The question, such as "Delete this project?" */
		title: string;
		/** What happens, such as "This can't be undone." */
		description?: string;
		confirmLabel?: string;
		cancelLabel?: string;
		/** Styles the confirm button for destructive actions. */
		danger?: boolean;
		/** Runs on confirm. If it returns a promise, the button shows a spinner until it settles, and the dialog stays open if it rejects. */
		onconfirm: () => void | Promise<unknown>;
		oncancel?: () => void;
		/** More content, such as a field to type a name into. Enter in one of its text fields confirms, once the confirm button is enabled. */
		children?: Snippet;
		/** Disables the confirm button, for example until a name is typed. */
		confirmDisabled?: boolean;
		class?: string;
	};
	let {
		open = $bindable(false),
		title,
		description,
		confirmLabel: confirmLabelProp,
		cancelLabel: cancelLabelProp,
		danger = false,
		onconfirm,
		oncancel,
		children,
		confirmDisabled = false,
		class: klass = ''
	}: Props = $props();
	const cancelLabel = $derived(cancelLabelProp ?? tb('tern.ui.alertDialog.cancel'));
	const confirmLabel = $derived(confirmLabelProp ?? tb('tern.ui.alertDialog.confirm'));

	const uid = $props.id();
	let busy = $state(false);
	let confirmed = false;

	async function confirm() {
		busy = true;
		try {
			await onconfirm();
			confirmed = true;
			open = false;
		} catch {
			// Stay open: the caller shows what went wrong.
		} finally {
			busy = false;
		}
	}

	// Enter in a text field confirms, as in a form: typing the name and pressing
	// Enter is deliberate. Buttons, text areas and IME composition keep Enter.
	function onkeydown(e: KeyboardEvent) {
		if (e.key !== 'Enter' || e.isComposing || e.defaultPrevented || e.shiftKey || e.altKey || e.ctrlKey || e.metaKey) return;
		if (!(e.target instanceof HTMLInputElement) || ['button', 'submit', 'reset', 'checkbox', 'radio', 'file'].includes(e.target.type)) return;
		e.preventDefault();
		if (!confirmDisabled && !busy) confirm();
	}
</script>

{#snippet body()}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="tern-alert-content" {onkeydown}>{@render children?.()}</div>
{/snippet}

<!-- Interrupts to confirm an action. Focus starts on Cancel, so Enter doesn't confirm by accident; Enter in a field inside it does. -->
<Dialog
	bind:open
	{title}
	{description}
	role="alertdialog"
	size="sm"
	dismissable={false}
	closeButton={false}
	initialFocus={() => document.getElementById(`tern-alert-${uid}-cancel`)}
	onclose={() => {
		if (!confirmed) oncancel?.();
		confirmed = false;
	}}
	class={klass}
	children={children ? body : undefined}
>
	{#snippet footer()}
		<Button variant="secondary" id="tern-alert-{uid}-cancel" onclick={() => (open = false)} disabled={busy}>{cancelLabel}</Button>
		<Button variant={danger ? 'danger' : 'primary'} onclick={confirm} loading={busy} disabled={confirmDisabled}>{confirmLabel}</Button>
	{/snippet}
</Dialog>

<style>
	.tern-alert-content {
		display: contents;
	}
</style>
