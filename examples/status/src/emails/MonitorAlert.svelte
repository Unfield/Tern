<script module lang="ts">
	// Props for the preview at /__tern/emails/MonitorAlert under tern dev.
	export const preview = {
		name: 'API',
		target: 'https://api.example.com/health',
		status: 'down',
		message: 'HTTP 503, expected 200-399',
		at: '2026-11-10T09:30:00Z',
		downtime: '',
		statusPage: 'https://status.example.com'
	};
</script>

<script lang="ts">
	let {
		name,
		target,
		status,
		message,
		at,
		downtime,
		statusPage
	}: { name: string; target: string; status: string; message: string; at: string; downtime: string; statusPage: string } = $props();
	const down = $derived(status === 'down');
</script>

<svelte:head>
	<title>{down ? `🔴 ${name} is down` : `🟢 ${name} is back up`}</title>
</svelte:head>

<div class="wrap">
	<div class="card">
		<p class="badge" class:down>{down ? 'Down' : 'Up'}</p>
		<h1>{name} {down ? 'is down' : 'is back up'}</h1>
		<p>
			{#if down}
				Our checks of <a href={target}>{target}</a> failed: <strong>{message}</strong>.
			{:else}
				<a href={target}>{target}</a> answers again{downtime ? ` after ${downtime}` : ''}.
			{/if}
		</p>
		<p class="muted">{at}</p>
		<p><a class="button" href={statusPage}>Open the status page</a></p>
	</div>
	<p class="muted footer">You get this because this address is a notification channel of the status page.</p>
</div>

<style>
	.wrap {
		font-family: system-ui, -apple-system, 'Segoe UI', sans-serif;
		background: #f4f4f5;
		padding: 24px;
		color: #18181b;
	}
	.card {
		max-width: 520px;
		margin: 0 auto;
		background: #fff;
		border-radius: 12px;
		padding: 24px;
	}
	h1 {
		font-size: 20px;
		margin: 8px 0 12px;
	}
	.badge {
		display: inline-block;
		margin: 0;
		padding: 2px 10px;
		border-radius: 999px;
		font-size: 12px;
		font-weight: 600;
		background: #dcfce7;
		color: #166534;
	}
	.badge.down {
		background: #fee2e2;
		color: #991b1b;
	}
	.muted {
		color: #71717a;
		font-size: 13px;
	}
	.button {
		display: inline-block;
		background: #18181b;
		color: #fff;
		text-decoration: none;
		padding: 8px 16px;
		border-radius: 8px;
	}
	.footer {
		text-align: center;
	}
</style>
