<script lang="ts">
	import type { Snippet } from 'svelte';
	import * as Card from '$lib/components/ui/card/index.js';
	import Logo from './Logo.svelte';

	let { title, description, children }: { title: string; description: string; children: Snippet } = $props();
</script>

<main class="bg-muted flex min-h-svh flex-col items-center justify-center gap-6 p-6">
	<Logo />
	<Card.Root class="w-full max-w-sm">
		<Card.Header>
			<Card.Title class="text-2xl">{title}</Card.Title>
			<Card.Description>{description}</Card.Description>
		</Card.Header>
		<Card.Content>{@render children()}</Card.Content>
	</Card.Root>
</main>
