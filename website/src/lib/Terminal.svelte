<script lang="ts">
	// A terminal that types its lines in when it scrolls into view, or shows
	// them at once for readers who prefer less motion.
	type Line = { text: string; kind?: 'cmd' | 'ok' | 'dim' | 'link' | 'blank'; note?: string };
	let { lines, title = 'Terminal' }: { lines: Line[]; title?: string } = $props();

	// Every line shows until the script starts typing them in.
	let shown = $state(Number.MAX_SAFE_INTEGER);
	let el = $state<HTMLElement>();

	$effect(() => {
		if (!el || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
		shown = 0;
		let timer: ReturnType<typeof setTimeout> | undefined;
		const io = new IntersectionObserver(
			(entries) => {
				if (!entries[0].isIntersecting) return;
				io.disconnect();
				const step = () => {
					shown++;
					if (shown < lines.length) timer = setTimeout(step, lines[shown - 1]?.kind === 'cmd' ? 520 : 240);
				};
				timer = setTimeout(step, 200);
			},
			{ threshold: 0.4 }
		);
		io.observe(el);
		return () => {
			io.disconnect();
			clearTimeout(timer);
		};
	});
</script>

<figure class="term" bind:this={el} aria-label={title}>
	<div class="chrome"><span class="dots" aria-hidden="true"><i></i><i></i><i></i></span><span class="title">{title}</span></div>
	<pre><code>{#each lines as l, i (i)}<span class="line {l.kind ?? ''}" class:hidden={i >= shown}>{#if l.kind === 'cmd'}<span class="prompt">$ </span>{/if}{#if l.kind === 'ok'}<span class="mark">✓ </span>{/if}{l.text}{#if l.note}<span class="note"> {l.note}</span>{/if}{#if l.kind === 'blank'}&nbsp;{/if}</span>{/each}</code></pre>
</figure>

<style>
	.term {
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
		height: 44px;
		padding: 0 var(--s-4);
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
	.title {
		color: var(--text-3);
		font-size: 12.5px;
	}
	pre {
		margin: 0;
		padding: var(--s-5) var(--s-5) var(--s-6);
		overflow-x: auto;
		font-size: 13px;
		line-height: 1.75;
		color: var(--text-2);
	}
	/* Long lines wrap under their text, not under the mark. */
	.line {
		display: block;
		padding-left: 2ch;
		text-indent: -2ch;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		transition: opacity 220ms var(--ease);
	}
	.hidden {
		opacity: 0;
	}
	.cmd {
		color: var(--text);
	}
	.prompt,
	.note,
	.dim {
		color: var(--text-3);
	}
	.mark {
		color: var(--accent);
	}
	.ok {
		color: var(--text);
	}
	.link {
		color: var(--accent);
	}
	@media (max-width: 640px) {
		pre {
			font-size: 12px;
			padding: var(--s-4);
		}
	}
</style>
