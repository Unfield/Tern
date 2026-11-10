<script lang="ts">
	import { SignIn } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>Sign in · Tern UI</title></svelte:head>

<div class="center">
	<SignIn
		description="Welcome back."
		providers={[
			{ name: 'github', label: 'GitHub' },
			{ name: 'google', label: 'Google' }
		]}
		rules={forms.default}
		{form}
		error={data.error || null}
		remember
		forgotHref="/auth/signin#forgot"
		signUpHref="/auth/signup"
	/>
</div>

<style>
	.center {
		display: grid;
		place-items: center;
		padding: 1rem 0;
	}
</style>
