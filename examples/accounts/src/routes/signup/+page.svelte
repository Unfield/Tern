<script lang="ts">
	import { SignUp } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { form }: PageProps = $props();
</script>

<svelte:head><title>Create an account · Accounts</title></svelte:head>

<SignUp rules={forms.default} {form} signInHref="/login" />
