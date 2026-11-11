<script lang="ts">
	import CopyButton from './CopyButton.svelte';

	let { command }: { command: string } = $props();
</script>

<div class="command">
	<span class="prompt" aria-hidden="true">$</span>
	<code>{command}</code>
	<CopyButton text={command} label="Copy the command" />
</div>

<style>
	.command {
		display: inline-flex;
		align-items: center;
		gap: var(--s-3);
		height: 44px;
		padding: 0 var(--s-1) 0 var(--s-4);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		background: var(--surface);
		font-size: 14.5px;
		box-shadow: var(--shadow);
	}
	.prompt {
		color: var(--text-3);
		font-family: var(--mono);
		user-select: none;
	}
	code {
		color: var(--text);
		white-space: nowrap;
	}
</style>
