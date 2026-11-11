<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import AuthCard from './AuthCard.svelte';
	import Button from './Button.svelte';
	import Field from './Field.svelte';
	import Form from './Form.svelte';
	import Input from './Input.svelte';

	type Props = {
		title?: string;
		description?: string;
		level?: 1 | 2 | 3;
		/** The action that calls acc.CreateTeam with the posted name. */
		action?: string;
		/** The page's form prop, for the action's errors. */
		form?: unknown;
		/** A link back, such as to the dashboard, for users who already have a team. */
		cancelHref?: string;
		class?: string;
	};
	let { title, description, level = 1, action = '?/create', form, cancelHref, class: klass = '' }: Props = $props();
	let submitting = $state(false);
</script>

<AuthCard id="tern-new-team-title" title={title ?? tb('tern.ui.team.new.title')} description={description ?? tb('tern.ui.team.new.description')} {level} class={klass}>
	<Form {action} {form} bind:submitting reset={false}>
		<Field label={tb('tern.ui.team.new.name')} name="name"><Input required maxlength={100} autocomplete="organization" /></Field>
		<Button type="submit" loading={submitting}>{tb('tern.ui.team.new.submit')}</Button>
	</Form>
	{#if cancelHref}<p class="tern-auth-note"><a href={cancelHref}>{tb('tern.ui.team.new.cancel')}</a></p>{/if}
</AuthCard>
