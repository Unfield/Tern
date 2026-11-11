<script lang="ts">
	import Icon from './Icon.svelte';

	let { text, label = 'Copy' }: { text: string; label?: string } = $props();
	let copied = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;

	async function copy() {
		try {
			await navigator.clipboard.writeText(text);
			copied = true;
			clearTimeout(timer);
			timer = setTimeout(() => (copied = false), 1600);
		} catch {
			// no clipboard access: nothing to do
		}
	}
</script>

<button type="button" class="copy" onclick={copy} aria-label={copied ? 'Copied' : label}>
	<Icon name={copied ? 'check' : 'copy'} size={15} />
</button>
<span class="visually-hidden" aria-live="polite">{copied ? 'Copied to the clipboard' : ''}</span>

<style>
	.copy {
		display: grid;
		place-items: center;
		width: 32px;
		height: 32px;
		border: 1px solid transparent;
		border-radius: var(--radius-s);
		background: transparent;
		color: var(--text-3);
		cursor: pointer;
		transition: color 160ms, background 160ms, border-color 160ms;
	}
	.copy:hover {
		color: var(--text);
		background: var(--surface-2);
		border-color: var(--border);
	}
</style>
