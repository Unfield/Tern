<script lang="ts">
	type Props = {
		/** The progress, from 0 to max; leave it out while the amount is unknown. */
		value?: number | null;
		max?: number;
		/** What is progressing, for screen readers and shown above the bar. */
		label: string;
		showLabel?: boolean;
		class?: string;
	};
	let { value = null, max = 100, label, showLabel = false, class: klass = '' }: Props = $props();
	const pct = $derived(value === null ? null : Math.max(0, Math.min(100, (value / max) * 100)));
	const uid = $props.id();
</script>

<div class="tern-progress {klass}">
	{#if showLabel}
		<div class="head">
			<span id="tern-progress-{uid}">{label}</span>
			{#if pct !== null}<span class="pct">{Math.round(pct)}%</span>{/if}
		</div>
	{/if}
	<div
		class="track"
		role="progressbar"
		aria-label={showLabel ? undefined : label}
		aria-labelledby={showLabel ? `tern-progress-${uid}` : undefined}
		aria-valuemin={0}
		aria-valuemax={max}
		aria-valuenow={value ?? undefined}
	>
		<div class="bar" class:indeterminate={pct === null} style:width={pct === null ? undefined : `${pct}%`}></div>
	</div>
</div>

<style>
	.head {
		display: flex;
		justify-content: space-between;
		margin-bottom: 0.35rem;
		font-size: 0.8125rem;
		font-weight: 550;
	}
	.pct {
		color: var(--tern-muted);
		font-variant-numeric: tabular-nums;
	}
	.track {
		height: 0.5rem;
		border-radius: 999px;
		background: var(--tern-surface-2);
		border: 1px solid var(--tern-border);
		overflow: hidden;
	}
	.bar {
		height: 100%;
		border-radius: inherit;
		background: var(--tern-accent);
		transition: width var(--tern-duration) ease-out;
	}
	.indeterminate {
		width: 35%;
		animation: slide 1.2s ease-in-out infinite;
	}
	@keyframes slide {
		from {
			translate: -100% 0;
		}
		to {
			translate: 300% 0;
		}
	}
</style>
