<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';

	type Props = {
		items: T[];
		/** Names the carousel, such as "Featured projects". */
		label: string;
		/** Draws one slide. */
		slide: Snippet<[T, number]>;
		/** Slides visible at once on wide screens. */
		perView?: number;
		/** Moves on by itself every this many milliseconds, with a pause button. Stops on hover and focus. */
		autoplay?: number;
		class?: string;
	};
	let { items, label, slide, perView = 1, autoplay, class: klass = '' }: Props = $props();

	const uid = $props.id();
	let track = $state<HTMLDivElement>();
	let current = $state(0);
	// svelte-ignore state_referenced_locally
	let playing = $state(!!autoplay);
	let paused = $state(false); // by hover or focus

	function go(i: number) {
		const n = Math.max(0, items.length - perView);
		current = ((i % (n + 1)) + n + 1) % (n + 1);
		const el = track?.children[current] as HTMLElement | undefined;
		track?.scrollTo({ left: el?.offsetLeft ?? 0, behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
	}

	$effect(() => {
		if (!autoplay || !playing || paused) return;
		const t = setInterval(() => go(current + 1), autoplay);
		return () => clearInterval(t);
	});

	// Keep current in step when the user scrolls or swipes.
	function onscroll() {
		if (!track) return;
		const w = (track.children[0] as HTMLElement | undefined)?.offsetWidth || 1;
		current = Math.round(track.scrollLeft / w);
	}
</script>

<section
	class="tern-carousel {klass}"
	aria-roledescription="carousel"
	aria-label={label}
	onpointerenter={() => (paused = true)}
	onpointerleave={() => (paused = false)}
	onfocusin={() => (paused = true)}
	onfocusout={() => (paused = false)}
	style:--per-view={perView}
>
	<div class="controls">
		{#if autoplay}
			<button type="button" class="control" aria-label={playing ? 'Stop slides' : 'Start slides'} onclick={() => (playing = !playing)}>
				{#if playing}
					<svg aria-hidden="true" viewBox="0 0 16 16" width="14" height="14"><path d="M5 3.5v9M11 3.5v9" stroke="currentColor" stroke-width="2" stroke-linecap="round" /></svg>
				{:else}
					<svg aria-hidden="true" viewBox="0 0 16 16" width="14" height="14"><path d="M5 3.5l7 4.5-7 4.5z" fill="currentColor" /></svg>
				{/if}
			</button>
		{/if}
		<button type="button" class="control" aria-label="Previous slide" aria-controls="tern-carousel-{uid}" onclick={() => go(current - 1)}>
			<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16"><path d="M10 4l-4 4 4 4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
		</button>
		<button type="button" class="control" aria-label="Next slide" aria-controls="tern-carousel-{uid}" onclick={() => go(current + 1)}>
			<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16"><path d="M6 4l4 4-4 4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
		</button>
	</div>
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div
		bind:this={track}
		class="track"
		id="tern-carousel-{uid}"
		role="group"
		aria-label="Slides"
		tabindex="0"
		aria-live={playing && !paused ? 'off' : 'polite'}
		{onscroll}
		onkeydown={(e) => {
			if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
				e.preventDefault();
				go(current + (e.key === 'ArrowRight' ? 1 : -1));
			}
		}}
	>
		{#each items as item, i (i)}
			<div class="slide" role="group" aria-roledescription="slide" aria-label="{i + 1} of {items.length}" inert={i < current || i >= current + perView ? true : undefined}>
				{@render slide(item, i)}
			</div>
		{/each}
	</div>
	<div class="dots" aria-hidden="true">
		{#each { length: Math.max(1, items.length - perView + 1) }, i (i)}<span class:on={i === current}></span>{/each}
	</div>
</section>

<style>
	.tern-carousel {
		display: grid;
		gap: 0.75rem;
	}
	.controls {
		display: flex;
		justify-content: flex-end;
		gap: 0.375rem;
	}
	.track {
		display: flex;
		gap: 1rem;
		overflow-x: auto;
		scroll-snap-type: x mandatory;
		scrollbar-width: none;
		border-radius: var(--tern-radius);
	}
	.track:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
	}
	.track::-webkit-scrollbar {
		display: none;
	}
	.slide {
		flex: 0 0 calc((100% - (var(--per-view) - 1) * 1rem) / var(--per-view));
		scroll-snap-align: start;
		min-width: 0;
	}
	@media (max-width: 40rem) {
		.slide {
			flex-basis: 85%;
		}
	}
	.control {
		display: grid;
		place-items: center;
		width: 2rem;
		height: 2rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border-strong);
		border-radius: 50%;
		cursor: pointer;
	}
	.control:hover {
		background: var(--tern-surface-2);
	}
	.control:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
	}
	.dots {
		display: flex;
		justify-content: center;
		gap: 0.375rem;
	}
	.dots span {
		width: 0.4rem;
		height: 0.4rem;
		border-radius: 50%;
		background: var(--tern-border-strong);
		transition: background var(--tern-duration);
	}
	.dots span.on {
		background: var(--tern-accent);
	}
</style>
