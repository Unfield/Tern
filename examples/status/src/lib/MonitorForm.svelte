<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import Field from './Field.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { Label } from '$lib/components/ui/label/index.js';

	type Values = {
		name: string;
		type: string;
		target: string;
		method: string;
		expectedStatus: string;
		keyword: string;
		intervalSeconds: number;
		timeoutSeconds: number;
		retries: number;
		certExpiryDays: number;
		group: string;
		public: boolean;
	};
	let {
		values,
		errors,
		groups,
		submit,
		cancel = '/admin'
	}: { values: Values; errors: Record<string, string> | null; groups: string[]; submit: string; cancel?: string } = $props();

	// svelte-ignore state_referenced_locally
	let type = $state(values.type);
	const err = (k: string) => errors?.[k];
	const types = [
		{ value: 'http', label: 'HTTP(S)', hint: 'A website or API answers with an expected status.' },
		{ value: 'tcp', label: 'TCP port', hint: 'A port accepts connections, such as a database.' },
		{ value: 'dns', label: 'DNS', hint: 'A host name resolves.' },
		{ value: 'tls', label: 'Certificate', hint: "A TLS certificate is valid and doesn't expire soon." }
	];
	const targetLabel = $derived(type === 'http' ? 'URL' : type === 'tcp' ? 'Host and port' : 'Host name');
	const targetPlaceholder = $derived(type === 'http' ? 'https://example.com/health' : type === 'tcp' ? 'db.internal:5432' : 'example.com');
</script>

<form method="POST" use:enhance class="grid gap-6">
	<Card.Root>
		<Card.Header><Card.Title>What to check</Card.Title></Card.Header>
		<Card.Content class="grid gap-5">
			<fieldset class="grid gap-2">
				<legend class="mb-2 text-sm font-medium">Type</legend>
				<div class="grid gap-2 sm:grid-cols-4">
					{#each types as t (t.value)}
						<label class="has-[:checked]:border-primary has-[:checked]:bg-muted/60 grid cursor-pointer gap-1 rounded-lg border p-3 text-sm">
							<span class="flex items-center gap-2 font-medium"><input type="radio" name="type" value={t.value} bind:group={type} />{t.label}</span>
							<span class="text-muted-foreground text-xs">{t.hint}</span>
						</label>
					{/each}
				</div>
				{#if err('type')}<p class="text-destructive text-sm">{err('type')}</p>{/if}
			</fieldset>
			<Field label="Name" name="name" value={values.name} error={err('name')} placeholder="Website" />
			<Field label={targetLabel} name="target" value={values.target} error={err('target')} placeholder={targetPlaceholder} />
			{#if type === 'http'}
				<div class="grid gap-4 sm:grid-cols-3">
					<div class="grid gap-2">
						<Label for="method">Method</Label>
						<select id="method" name="method" class="border-input bg-background h-9 rounded-md border px-3 text-sm">
							{#each ['GET', 'HEAD', 'POST'] as m (m)}<option selected={values.method === m}>{m}</option>{/each}
						</select>
					</div>
					<Field label="Accepted status codes" name="expectedStatus" value={values.expectedStatus} error={err('expectedStatus')} hint="Such as 200-299,301" />
					<Field label="Keyword" name="keyword" value={values.keyword} hint="Optional: the response must contain it" />
				</div>
			{/if}
			{#if type === 'tls'}
				<Field label="Warn when the certificate expires within (days)" name="certExpiryDays" type="number" min="1" value={values.certExpiryDays} />
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header><Card.Title>How often</Card.Title></Card.Header>
		<Card.Content class="grid gap-4 sm:grid-cols-3">
			<Field label="Interval (seconds)" name="intervalSeconds" type="number" min="5" value={values.intervalSeconds} error={err('intervalSeconds')} />
			<Field label="Timeout (seconds)" name="timeoutSeconds" type="number" min="1" value={values.timeoutSeconds} error={err('timeoutSeconds')} />
			<Field label="Retries before down" name="retries" type="number" min="0" value={values.retries} error={err('retries')} hint="Failed checks tolerated first" />
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header><Card.Title>Status page</Card.Title></Card.Header>
		<Card.Content class="grid gap-4 sm:grid-cols-2">
			<Field label="Group" name="group" value={values.group} list="groups" />
			<datalist id="groups">{#each groups as g (g)}<option value={g}></option>{/each}</datalist>
			<label class="flex items-center gap-2 self-end pb-2 text-sm">
				<input type="checkbox" name="public" checked={values.public} /> Show on the public status page
			</label>
		</Card.Content>
	</Card.Root>

	<div class="flex justify-end gap-2">
		<Button variant="outline" href={cancel}>Cancel</Button>
		<Button type="submit">{submit}</Button>
	</div>
</form>
