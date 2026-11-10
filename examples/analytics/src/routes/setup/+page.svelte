<script lang="ts">
	import { SignUp } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { form }: PageProps = $props();
</script>

<svelte:head><title>Set up · Analytics</title></svelte:head>

<main class="center">
	<SignUp title="Set up your analytics" description="Create the owner account. You can add sites next." rules={forms.default} {form} />
</main>

<style>
	.center {
		display: grid;
		place-items: center;
		min-height: 100vh;
		padding: 1rem;
		box-sizing: border-box;
	}
</style>
