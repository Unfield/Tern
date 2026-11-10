<script lang="ts">
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';

	interface Props {
		label: string;
		name: string;
		error?: string;
		hint?: string;
		value?: string | number | null;
		type?: string;
		placeholder?: string;
		autocomplete?: AutoFill | null;
		min?: string;
		list?: string;
	}
	let { label, name, error, hint, ...rest }: Props = $props();
	const id = $props.id();
</script>

<div class="grid gap-2">
	<Label for={id}>{label}</Label>
	<Input id={id} {name} aria-invalid={error ? true : undefined} aria-describedby={error || hint ? `${id}-note` : undefined} {...rest} />
	{#if error}
		<p id="{id}-note" class="text-destructive text-sm" role="alert">{error}</p>
	{:else if hint}
		<p id="{id}-note" class="text-muted-foreground text-xs">{hint}</p>
	{/if}
</div>
