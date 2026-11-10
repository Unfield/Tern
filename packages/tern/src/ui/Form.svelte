<script lang="ts">
	import { tick, type Snippet } from 'svelte';
	import type { HTMLFormAttributes } from 'svelte/elements';
	import { enhance as enhanceAction } from '../runtime/forms.js';
	import { validate, type FormField } from '../runtime/validate.js';
	import Alert from './Alert.svelte';
	import { setForm } from './form.js';

	type Failure = { errors?: Record<string, string>; values?: Record<string, unknown>; message?: string };

	type Props = Omit<HTMLFormAttributes, 'onsubmit'> & {
		/** The action's rules, from ./$forms: forms.create. The browser checks them as the user types and before posting. */
		rules?: FormField[];
		/** The page's form prop, so the server's errors and the posted values show up. */
		form?: unknown;
		/** Post in the background with use:enhance. Without it, or without JavaScript, the browser posts the form. */
		enhance?: boolean;
		/** Whether a post is in flight; bind it to show a spinner. */
		submitting?: boolean;
		/** Clear the form after a successful post. */
		reset?: boolean;
		/** Runs after a successful post with the action's result. */
		onsuccess?: (data: unknown) => void;
		children: Snippet;
	};
	let { rules = [], form, enhance = true, submitting = $bindable(false), reset = true, onsuccess, method = 'POST', children, class: klass = '', ...rest }: Props = $props();

	const uid = $props.id();
	let el = $state<HTMLFormElement>();
	let client = $state<Record<string, string>>({});
	let touched = $state<Record<string, boolean>>({});
	let edited = $state<Record<string, boolean>>({});
	let submitted = $state(false);
	let hydrated = $state(false);
	let summary = $state('');

	const failure = $derived((form && typeof form === 'object' && 'errors' in form ? form : null) as Failure | null);
	// A new answer from the server replaces what the user had edited since the last one.
	$effect(() => {
		void failure;
		edited = {};
	});
	$effect(() => {
		hydrated = true;
	});

	const byName = $derived(new Map(rules.map((f) => [f.name, f])));
	setForm({
		error: (name) => client[name] ?? (edited[name] ? undefined : failure?.errors?.[name]),
		value: (name) => {
			const v = failure?.values?.[name];
			return typeof v === 'string' ? v : undefined;
		},
		field: (name) => byName.get(name)
	});

	function check(names: string[]) {
		if (!el || rules.length === 0) return;
		const all = validate(rules, new FormData(el));
		const next = { ...client };
		for (const n of names) {
			if (all[n]) next[n] = all[n]!;
			else delete next[n];
		}
		client = next;
	}
	// A field and the fields that must match it, such as a password and its confirmation.
	const related = (name: string) => [name, ...rules.filter((f) => f.rules?.some((r) => r.name === 'same' && r.arg === name)).map((f) => f.name)];

	function oninput(e: Event) {
		const name = (e.target as HTMLInputElement | null)?.name;
		if (!name) return;
		edited = { ...edited, [name]: true };
		if (touched[name] || submitted) check(related(name).filter((n) => touched[n] || submitted || n === name));
	}
	function onfocusout(e: FocusEvent) {
		const name = (e.target as HTMLInputElement | null)?.name;
		// Don't complain about a field the user only passed through.
		if (!name || !byName.has(name) || (!edited[name] && !submitted)) return;
		touched = { ...touched, [name]: true };
		check([name]);
	}

	/** Checks everything; returns false and focuses the first problem if anything is wrong. */
	function checkAll(data: FormData) {
		submitted = true;
		const errors = validate(rules, data) as Record<string, string>;
		client = errors;
		const names = Object.keys(errors);
		if (names.length === 0) {
			summary = '';
			return true;
		}
		summary = names.length === 1 ? '1 field needs attention.' : `${names.length} fields need attention.`;
		void tick().then(() => el?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus());
		return false;
	}

	const submit = ({ formData, cancel }: { formData: FormData; cancel(): void }) => {
		const errors = validate(rules, formData);
		if (Object.keys(errors).length > 0) {
			cancel();
			checkAll(formData);
			return;
		}
		submitted = true;
		client = {};
		summary = '';
		submitting = true;
		return async ({ update, result }: { update: (o?: { reset?: boolean }) => Promise<void>; result: { type: string; data?: unknown } }) => {
			await update({ reset });
			submitting = false;
			if (result.type === 'success') {
				submitted = false;
				touched = {};
				onsuccess?.(result.data);
			} else if (result.type === 'failure') {
				await tick();
				el?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
			}
		};
	};

	function attach(node: HTMLFormElement) {
		if (!enhance) return;
		const action = enhanceAction(node, submit as never);
		return () => action?.destroy?.();
	}
</script>

<form
	bind:this={el}
	class="tern-form {klass}"
	{method}
	novalidate={hydrated && rules.length > 0}
	aria-describedby={failure?.message ? `tern-form-${uid}-message` : undefined}
	{...rest}
	{oninput}
	onchange={oninput}
	{onfocusout}
	onsubmit={(e) => {
		if (enhance) return; // use:enhance checks first
		if (!checkAll(new FormData(e.currentTarget))) e.preventDefault();
	}}
	{@attach attach}
>
	{#if failure?.message}
		<div id="tern-form-{uid}-message"><Alert tone="danger" live>{failure.message}</Alert></div>
	{/if}
	{@render children()}
	<p class="tern-sr-only" role="status">{summary}</p>
</form>

<style>
	.tern-form {
		display: grid;
		gap: 1rem;
	}
</style>
