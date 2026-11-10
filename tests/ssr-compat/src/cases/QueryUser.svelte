<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	const query = createQuery(() => ({ queryKey: ['user'], queryFn: async () => ({ name: 'never' }) }));
</script>

<p>user: {query.data?.name ?? 'loading'}</p>
