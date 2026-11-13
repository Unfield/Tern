<script lang="ts">
	import { Admin } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
</script>

<Admin {data} {form} />
