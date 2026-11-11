<script lang="ts">
	import CopyButton from './CopyButton.svelte';

	type Sample = { file: string; html: string; code: string };
	let { samples, label }: { samples: Sample[]; label: string } = $props();
	let current = $state(0);
	const id = 'code-' + Math.random().toString(36).slice(2, 8);

	function name(file: string) {
		return file.split('/').pop() ?? file;
	}
	function dir(file: string) {
		const i = file.lastIndexOf('/');
		return i < 0 ? '' : file.slice(0, i + 1);
	}
	function onKey(e: KeyboardEvent) {
		if (e.key === 'ArrowRight') current = (current + 1) % samples.length;
		else if (e.key === 'ArrowLeft') current = (current - 1 + samples.length) % samples.length;
		else return;
		e.preventDefault();
		(e.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('[role=tab]')[current]?.focus();
	}
</script>

<figure class="window" aria-label={label}>
	<div class="chrome">
		<span class="dots" aria-hidden="true"><i></i><i></i><i></i></span>
		{#if samples.length > 1}
			<div class="tabs" role="tablist" tabindex="-1" aria-label="Files" onkeydown={onKey}>
				{#each samples as s, i (s.file)}
					<button type="button" role="tab" id="{id}-tab-{i}" aria-selected={i === current} aria-controls="{id}-panel" tabindex={i === current ? 0 : -1} onclick={() => (current = i)}>
						{name(s.file)}
					</button>
				{/each}
			</div>
		{:else}
			<span class="single">{name(samples[0].file)}</span>
		{/if}
		<span class="spacer"></span>
		<CopyButton text={samples[current].code} label="Copy the code" />
	</div>
	{#if dir(samples[current].file)}
		<div class="path">{dir(samples[current].file)}<b>{name(samples[current].file)}</b></div>
	{/if}
	<!-- svelte-ignore a11y_no_noninteractive_tabindex: a code block wider than its box must be reachable by keyboard to scroll it -->
	<pre id="{id}-panel" role={samples.length > 1 ? 'tabpanel' : undefined} aria-labelledby={samples.length > 1 ? `${id}-tab-${current}` : undefined} tabindex="0"><code class="chroma">{@html samples[current].html}</code></pre>
</figure>

<style>
	.window {
		margin: 0;
		min-width: 0;
		border: 1px solid var(--border);
		border-radius: var(--radius-l);
		background: var(--code-bg);
		box-shadow: var(--shadow-l);
		overflow: hidden;
	}
	.chrome {
		display: flex;
		align-items: center;
		gap: var(--s-4);
		height: 48px;
		padding: 0 var(--s-2) 0 var(--s-4);
		border-bottom: 1px solid var(--border);
		background: var(--surface);
	}
	.dots {
		display: flex;
		gap: 6px;
	}
	.dots i {
		width: 11px;
		height: 11px;
		border-radius: 50%;
		background: var(--border-strong);
	}
	.tabs {
		display: flex;
		gap: var(--s-1);
		min-width: 0;
		overflow-x: auto;
		scrollbar-width: none;
	}
	.tabs button {
		height: 30px;
		padding: 0 var(--s-3);
		border: 1px solid transparent;
		border-radius: var(--radius-s);
		background: transparent;
		color: var(--text-3);
		font-family: var(--mono);
		font-size: 12.5px;
		white-space: nowrap;
		cursor: pointer;
		transition: color 160ms, background 160ms;
	}
	.tabs button:hover {
		color: var(--text);
	}
	.tabs button[aria-selected='true'] {
		color: var(--text);
		background: var(--surface-2);
		border-color: var(--border);
	}
	.single {
		color: var(--text-2);
		font-family: var(--mono);
		font-size: 12.5px;
	}
	.spacer {
		flex: 1;
	}
	.path + pre {
		padding-top: var(--s-3);
	}
	.path {
		padding: var(--s-4) var(--s-5) 0;
		color: var(--text-3);
		font-family: var(--mono);
		font-size: 12px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.path b {
		color: var(--text-2);
		font-weight: 500;
	}
	pre {
		margin: 0;
		padding: var(--s-5) var(--s-5) var(--s-6);
		scrollbar-width: thin;
		scrollbar-color: var(--border-strong) transparent;
		overflow-x: auto;
		font-size: 13px;
		line-height: 1.7;
		tab-size: 4;
	}
	pre:focus-visible {
		outline-offset: -2px;
	}
	@media (max-width: 640px) {
		.dots {
			display: none;
		}
		.chrome {
			gap: var(--s-2);
			padding-left: var(--s-2);
		}
		pre {
			font-size: 12px;
			padding: var(--s-3) var(--s-4) var(--s-5);
		}
		.path {
			padding: var(--s-3) var(--s-4) 0;
		}
	}
</style>
