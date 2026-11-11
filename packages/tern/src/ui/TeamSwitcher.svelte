<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import Menu from './Menu.svelte';
	import type { MenuItem } from './menu.js';

	type Membership = { team: { id: number; name: string }; role: string };
	type Props = {
		/** From acc.Teams. */
		teams: Membership[];
		/** The current team's ID, from accounts.CurrentTeam. */
		current: number;
		/** The action that calls acc.SwitchTeam with the posted id. */
		action: string;
		/** A link to make a new team, such as "/teams/new". */
		newHref?: string;
		/** Opens to the start or end edge of the button. */
		align?: 'start' | 'end';
		class?: string;
	};
	let { teams, current, action, newHref, align = 'start', class: klass = '' }: Props = $props();

	let form = $state<HTMLFormElement>();
	let chosen = $state('');
	const name = $derived(teams.find((m) => m.team.id === current)?.team.name ?? tb('tern.ui.team.switcher.none'));
	const items = $derived<MenuItem[]>([
		{ heading: tb('tern.ui.team.switcher.heading') },
		...teams.map((m) => ({
			label: m.team.name,
			type: 'radio' as const,
			checked: m.team.id === current,
			onselect: () => {
				if (m.team.id === current) return;
				chosen = String(m.team.id);
				// After the hidden input has the value.
				queueMicrotask(() => form?.requestSubmit());
			}
		})),
		...(newHref ? [{ separator: true }, { label: tb('tern.ui.team.switcher.new'), href: newHref }] : [])
	]);
</script>

<!-- Posts the chosen team to the switch action; the page reloads its data. -->
<form method="POST" {action} bind:this={form} hidden>
	<input type="hidden" name="id" value={chosen} />
</form>
<Menu {items} {align} variant="ghost" class="tern-team-switcher {klass}">
	<span class="tern-team-switcher-name">{name}</span>
</Menu>

<style>
	.tern-team-switcher-name {
		max-width: 14rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-weight: 600;
	}
</style>
