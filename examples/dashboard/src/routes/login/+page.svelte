<script lang="ts">
	import { enhance, Image, t } from '@ternkit/tern';
	import artwork from '$lib/assets/login.jpg?image';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import type { PageProps } from './$types';

	let { form }: PageProps = $props();
	let submitting = $state(false);
</script>

<svelte:head>
	<title>{t('login.title')} · Acme Dashboard</title>
</svelte:head>

<main class="grid min-h-svh lg:grid-cols-2">
	<Image
		src={artwork}
		alt=""
		sizes="50vw"
		priority
		class="hidden h-full max-h-svh w-full object-cover lg:block"
		data-testid="artwork"
	/>
	<div class="bg-muted flex items-center justify-center p-6">
		<Card.Root class="w-full max-w-sm">
			<Card.Header>
				<Card.Title class="text-2xl">{t('login.title')}</Card.Title>
				<Card.Description>{t('login.description')}</Card.Description>
			</Card.Header>
			<Card.Content>
				<form
					method="POST"
					class="grid gap-4"
					use:enhance={() => {
						submitting = true;
						return async ({ update }) => {
							await update();
							submitting = false;
						};
					}}
				>
					<div class="grid gap-2">
						<Label for="name">{t('login.name')}</Label>
						<Input id="name" name="name" value={form?.name ?? ''} autocomplete="name" aria-invalid={form?.error ? true : undefined} />
						{#if form?.error}<p class="text-destructive text-sm" role="alert">{form.error}</p>{/if}
					</div>
					<Button type="submit" disabled={submitting}>{submitting ? t('login.submitting') : t('login.submit')}</Button>
				</form>
			</Card.Content>
		</Card.Root>
	</div>
</main>
