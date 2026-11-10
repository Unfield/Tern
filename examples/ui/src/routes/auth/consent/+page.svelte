<script lang="ts">
	import { ConsentScreen } from '@ternkit/tern/ui';

	// A real page loads this with provider.ConsentRequest(ctx) from plugins/oidc.
	const consent = {
		client: 'Team wiki',
		scopes: ['Know who you are', 'See your name and picture', 'See your email address'],
		action: '/auth/consent#posted',
		params: { client_id: 'wiki', state: 'xyz' }
	};
</script>

<svelte:head><title>Allow Team wiki? · Tern UI</title></svelte:head>

<div class="center">
	<ConsentScreen {consent} user={{ name: 'Ada Lovelace', email: 'ada@example.com' }} switchHref="/auth/signin" />
</div>

<style>
	.center {
		display: grid;
		place-items: center;
		padding: 1rem 0;
	}
</style>
