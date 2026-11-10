<script lang="ts">
	import { SignIn } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>Sign in · Analytics</title></svelte:head>

<main class="center">
	<SignIn description="See how your sites are doing." rules={forms.default} {form} next={data.next || undefined} />
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
