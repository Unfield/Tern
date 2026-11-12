<script lang="ts">
	import { SignIn } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>Sign in · Shelf</title></svelte:head>

<SignIn rules={forms.default} {form} next={data.next} remember signUpHref="/signup" />
