<script lang="ts">
	import { api } from '$api';
	import {
		Button,
		ButtonGroup,
		Calendar,
		Combobox,
		Command,
		DatePicker,
		Field,
		Input,
		InputGroup,
		InputOTP,
		Kbd,
		Label,
		Menu,
		PasswordInput,
		SelectMenu,
		Slider,
		Toggle,
		ToggleGroup,
		toast,
		type DateRange,
		type Option
	} from '@ternkit/tern/ui';

	const languages: Option[] = [
		{ value: 'go', label: 'Go', group: 'Compiled' },
		{ value: 'rust', label: 'Rust', group: 'Compiled' },
		{ value: 'zig', label: 'Zig', group: 'Compiled', disabled: true },
		{ value: 'ts', label: 'TypeScript', group: 'Scripting' },
		{ value: 'py', label: 'Python', group: 'Scripting' },
		{ value: 'rb', label: 'Ruby', group: 'Scripting' }
	];
	let language = $state<string | null>(null);

	let people = $state<Option[]>([]);
	let owner = $state<string | null>(null);
	async function search(q: string) {
		const found = await api('GET /api/people', { query: { q } });
		people = (found ?? []).map((p) => ({ value: p.id, label: p.name, description: p.email }));
	}

	let region = $state<string | null>('eu');
	const regions: Option[] = [
		{ value: 'eu', label: 'Europe', description: 'Frankfurt and Helsinki' },
		{ value: 'us', label: 'United States', description: 'Ashburn and Hillsboro' },
		{ value: 'ap', label: 'Asia Pacific', description: 'Singapore' }
	];

	let due = $state<string | null>(null);
	let trip = $state<DateRange>({ start: null, end: null });
	let day = $state<string | null>('2026-11-10');

	let volume = $state(40);
	let price = $state<[number, number]>([20, 80]);
	let bold = $state(false);
	let align = $state<string | null>('left');
	let styles = $state<string[]>(['bold']);
	let code = $state('');
	let password = $state('');
	let ran = $state('');
	let paletteOpen = $state(false);

	const commands = [
		{ label: 'New project', group: 'Projects', shortcut: '⌘N', onselect: () => (ran = 'New project') },
		{ label: 'Open project…', group: 'Projects', onselect: () => (ran = 'Open project') },
		{ label: 'Settings', group: 'App', keywords: ['preferences', 'options'], shortcut: '⌘,', onselect: () => (ran = 'Settings') },
		{ label: 'Toggle dark mode', group: 'App', keywords: ['theme'], onselect: () => (ran = 'Toggle dark mode') },
		{ label: 'Sign out', group: 'Account', onselect: () => (ran = 'Sign out') }
	];
</script>

<svelte:head><title>Inputs · Tern UI</title></svelte:head>

<h1>Inputs</h1>
<p class="lead">Every control fits in a <code>Field</code>, which labels it, describes it and shows its error. They post with forms by <code>name</code>.</p>

<section aria-labelledby="choose">
	<h2 id="choose">Choosing</h2>
	<div class="grid">
		<Field label="Language" description="Filters as you type.">
			<Combobox options={languages} bind:value={language} placeholder="Search languages" name="language" />
		</Field>
		<Field label="Owner" description="Searches a Go endpoint.">
			<Combobox options={people} bind:value={owner} onsearch={search} placeholder="Search people" empty="Nobody found" />
		</Field>
		<Field label="Region">
			<SelectMenu options={regions} bind:value={region} name="region" />
		</Field>
	</div>
	<p data-testid="chosen">Language: {language ?? 'none'} · owner: {owner ?? 'none'} · region: {region ?? 'none'}</p>
</section>

<section aria-labelledby="dates">
	<h2 id="dates">Dates</h2>
	<div class="grid">
		<Field label="Due date">
			<DatePicker bind:value={due} name="due" min="2026-01-01" />
		</Field>
		<Field label="Trip">
			<DatePicker mode="range" bind:value={trip} />
		</Field>
	</div>
	<div class="row">
		<Calendar bind:value={day} label="Pick a day" isDisabled={(d) => [0, 6].includes(new Date(`${d}T00:00:00Z`).getUTCDay())} weekStart={1} locale="en-GB" />
	</div>
	<p data-testid="dates">Due: {due ?? 'none'} · trip: {trip.start ?? '…'} to {trip.end ?? '…'} · day: {day}</p>
</section>

<section aria-labelledby="ranges">
	<h2 id="ranges">Sliders</h2>
	<div class="grid">
		<Field label="Volume">
			<Slider bind:value={volume} format={(v) => `${v}%`} name="volume" />
		</Field>
		<Field label="Price">
			<Slider bind:value={price} min={0} max={200} step={5} format={(v) => `€${v}`} />
		</Field>
	</div>
	<p data-testid="sliders">Volume {volume} · price {price[0]} to {price[1]}</p>
</section>

<section aria-labelledby="toggles">
	<h2 id="toggles">Toggles and groups</h2>
	<div class="row">
		<Toggle bind:pressed={bold} aria-label="Bold"><strong>B</strong></Toggle>
		<ToggleGroup label="Alignment" bind:value={align} options={['left', 'center', 'right'].map((v) => ({ value: v, label: v[0]!.toUpperCase() + v.slice(1) }))} />
		<ToggleGroup
			label="Text style"
			type="multiple"
			bind:value={styles}
			options={[
				{ value: 'bold', label: 'Bold' },
				{ value: 'italic', label: 'Italic' },
				{ value: 'underline', label: 'Underline' }
			]}
		/>
	</div>
	<div class="row">
		<ButtonGroup label="Pagination">
			<Button variant="secondary">Previous</Button>
			<Button variant="secondary">1</Button>
			<Button variant="secondary">2</Button>
			<Button variant="secondary">Next</Button>
		</ButtonGroup>
		<ButtonGroup label="Deploy">
			<Button>Deploy</Button>
			<Menu items={[{ label: 'Deploy to staging' }, { label: 'Deploy without cache' }]} variant="secondary">More</Menu>
		</ButtonGroup>
	</div>
	<p data-testid="toggles">Bold {bold ? 'on' : 'off'} · align {align ?? 'none'} · styles {styles.join(', ') || 'none'}</p>
</section>

<section aria-labelledby="text">
	<h2 id="text">Text</h2>
	<div class="grid">
		<Field label="Website" description="Your site's address, without https://.">
			<InputGroup>
				{#snippet start()}https://{/snippet}
				<Input name="site" placeholder="example.com" />
			</InputGroup>
		</Field>
		<Field label="Invite link">
			<InputGroup>
				<Input value="https://tern.example/join/7Hq2" readonly />
				{#snippet end()}<Button size="sm" variant="ghost" onclick={() => toast('Link copied')}>Copy</Button>{/snippet}
			</InputGroup>
		</Field>
		<Field label="Password" description="At least 12 characters.">
			<PasswordInput bind:value={password} autocomplete="new-password" name="password" data-testid="password" />
		</Field>
		<Field label="Verification code" description="We sent a 6-digit code to your phone.">
			<InputOTP bind:value={code} oncomplete={(c) => toast.success(`Code ${c} entered`)} />
		</Field>
	</div>
	<div class="row">
		<Label for="standalone">A standalone label</Label>
		<Input id="standalone" style="max-width: 14rem" />
	</div>
	<p data-testid="text">Code: {code || 'none'} · password length {password.length}</p>
</section>

<section aria-labelledby="command">
	<h2 id="command">Command palette</h2>
	<div class="command">
		<Command items={commands} label="Commands" />
	</div>
	<p>
		Or press <Kbd>⌘</Kbd> <Kbd>K</Kbd>, or
		<Button size="sm" variant="secondary" onclick={() => (paletteOpen = true)} data-testid="open-palette">open the palette</Button>.
	</p>
	<Command items={commands} dialog bind:open={paletteOpen} hotkey="k" label="Command palette" />
	<p data-testid="ran">Ran: {ran || 'nothing yet'}</p>
</section>

<style>
	.lead {
		color: var(--tern-muted);
	}
	section {
		margin-top: 2.5rem;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
		gap: 1.25rem;
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1rem;
	}
	.command {
		max-width: 28rem;
	}
</style>
