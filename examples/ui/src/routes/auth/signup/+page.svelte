<script lang="ts">
	import { SignUp } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { form }: PageProps = $props();
</script>

<svelte:head><title>Create an account · Tern UI</title></svelte:head>

<div class="center">
	<SignUp rules={forms.default} {form} confirm termsHref="/auth/signup#terms" signInHref="/auth/signin" />
</div>

<style>
	.center {
		display: grid;
		place-items: center;
		padding: 1rem 0;
	}
</style>
