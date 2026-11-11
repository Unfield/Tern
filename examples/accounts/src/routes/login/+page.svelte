<script lang="ts">
	import { SignIn } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>Sign in · Accounts</title></svelte:head>

<SignIn rules={forms.default} {form} next={data.next} passkey remember forgotHref="/forgot-password" signUpHref="/signup" />
