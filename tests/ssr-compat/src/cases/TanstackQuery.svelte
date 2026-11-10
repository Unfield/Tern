<script lang="ts">
	import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
	import QueryUser from './QueryUser.svelte';
	const client = new QueryClient({ defaultOptions: { queries: { staleTime: 60_000 } } });
	client.setQueryData(['user'], { name: 'Ada' });
</script>

<QueryClientProvider {client}><QueryUser /></QueryClientProvider>
