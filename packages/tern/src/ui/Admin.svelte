<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import { enhance } from '../runtime/forms.js';
	import Alert from './Alert.svelte';
	import AlertDialog from './AlertDialog.svelte';
	import Button from './Button.svelte';
	import Checkbox from './Checkbox.svelte';
	import Empty from './Empty.svelte';
	import Field from './Field.svelte';
	import Form from './Form.svelte';
	import Input from './Input.svelte';
	import Pagination from './Pagination.svelte';
	import Select from './Select.svelte';
	import Textarea from './Textarea.svelte';
	import type { FormField } from '../runtime/validate.js';
	import { listHref, shown, type AdminField, type AdminPage } from './admin.js';

	type Props = {
		/** The page's data, from admin.Load. */
		data: AdminPage;
		/** The page's form prop, for the form's errors. */
		form?: unknown;
		class?: string;
	};
	let { data, form, class: klass = '' }: Props = $props();

	type Failure = { errors?: Record<string, string>; values?: Record<string, unknown>; message?: string };
	const failure = $derived((form && typeof form === 'object' && 'errors' in form ? form : null) as Failure | null);

	const res = $derived(data.resource);
	const list = $derived(data.list);
	const record = $derived(data.record);
	const lower = (s: string) => s.toLowerCase();
	const yes = $derived(tb('tern.admin.yes'));
	const no = $derived(tb('tern.admin.no'));

	const listed = $derived(res ? res.fields.filter((f) => f.list) : []);
	// Besides the first column, the first text one opens the record: a title is a bigger target than an ID.
	const linkField = $derived(listed.find((f) => f.input === 'text')?.name);
	const filters = $derived(res ? res.fields.filter((f) => f.filter && (f.input === 'checkbox' || f.options)) : []);
	const editing = $derived(!!record?.key);
	// Creating, the fields the database fills in are left out.
	const fields = $derived(res ? res.fields.filter((f) => editing || (f.name === res.key ? f.rules.kind === 'string' : !f.readonly)) : []);
	const editable = $derived(!!res && (editing ? res.edit : res.create));

	const value = (f: AdminField) => {
		const v = failure?.values?.[f.name];
		return typeof v === 'string' ? v : (record?.values[f.name] ?? '');
	};
	const notice = $derived.by(() => {
		if (data.notice === 'created' || data.notice === 'saved' || data.notice === 'deleted') return tb(`tern.admin.done.${data.notice}`);
		const act = res?.actions?.find((a) => a.name === data.notice);
		return act ? tb('tern.admin.done.action', { label: act.label }) : '';
	});
	// The record's action waiting for its confirmation, and its form.
	let asking = $state<{ name: string; label: string } | null>(null);
	let actionForms = $state<Record<string, HTMLFormElement>>({});
	const from = $derived(list && list.total ? (list.page - 1) * list.perPage + 1 : 0);

	let confirming = $state(false);
	let deleteForm = $state<HTMLFormElement>();
	const heading = $derived(
		data.view === 'home' ? tb('tern.admin.home') : data.view === 'list' ? res!.label : editing ? tb('tern.admin.edit', { name: lower(res!.singular) }) : tb('tern.admin.new', { name: lower(res!.singular) })
	);
</script>

<svelte:head><title>{heading} · {data.title}</title></svelte:head>

<div class="tern-admin {klass}">
	<nav class="tern-admin-nav" aria-label={tb('tern.admin.menu')}>
		<a class="tern-admin-title" href={data.base}>{data.title}</a>
		<ul>
			<li><a href={data.base} aria-current={data.view === 'home' ? 'page' : undefined}>{tb('tern.admin.home')}</a></li>
			{#each data.resources as r (r.name)}
				<li><a href={r.href} aria-current={res?.name === r.name ? (data.view === 'list' ? 'page' : 'true') : undefined}>{r.label}</a></li>
			{/each}
		</ul>
	</nav>

	<main class="tern-admin-main">
		{#if data.view === 'home'}
			<h1>{heading}</h1>
			<ul class="tern-admin-cards">
				{#each data.resources as r, i (r.name)}
					<li>
						<a href={r.href}>
							<span class="tern-admin-card-label">{r.label}</span>
							<span class="tern-admin-card-count">{tb('tern.admin.count', { count: data.counts[i] ?? 0 })}</span>
						</a>
					</li>
				{/each}
			</ul>
		{:else if data.view === 'list' && res && list}
			<header class="tern-admin-head">
				<h1>{heading}</h1>
				{#if res.create}<Button href="{res.href}/new">{tb('tern.admin.new', { name: lower(res.singular) })}</Button>{/if}
			</header>
			{#if notice}<Alert tone="success" live>{notice}</Alert>{/if}

			{#if res.search || filters.length}
				<form class="tern-admin-query" method="GET" action={res.href} role="search">
					{#if res.search}
						<Input type="search" name="q" value={list.search} aria-label={tb('tern.admin.searchLabel', { name: lower(res.label) })} placeholder={tb('tern.admin.search')} />
					{/if}
					{#each filters as f (f.name)}
						<Select
							name={f.name}
							aria-label={f.label}
							value={list.filters[f.name] ?? ''}
							options={[
								{ value: '', label: `${f.label}: ${tb('tern.admin.all')}` },
								...(f.input === 'checkbox'
									? [
											{ value: 'true', label: `${f.label}: ${yes}` },
											{ value: 'false', label: `${f.label}: ${no}` }
										]
									: (f.options ?? []).map((o) => ({ value: o, label: `${f.label}: ${o}` })))
							]}
						/>
					{/each}
					{#if list.sort}<input type="hidden" name="sort" value="{list.desc ? '-' : ''}{list.sort}" />{/if}
					<Button type="submit" variant="secondary">{tb('tern.admin.search')}</Button>
					{#if list.search || Object.keys(list.filters).length}
						<Button href={res.href} variant="ghost">{tb('tern.admin.clear')}</Button>
					{/if}
				</form>
			{/if}

			{#if list.rows.length === 0}
				<Empty
					level={2}
					title={list.search || Object.keys(list.filters).length ? tb('tern.admin.noMatches') : tb('tern.admin.empty', { name: lower(res.label) })}
				/>
			{:else}
				<div class="tern-admin-scroll">
					<table class="tern-admin-table">
						<caption class="tern-sr-only">{res.label}</caption>
						<thead>
							<tr>
								{#each listed as f (f.name)}
									{@const active = list.sort === f.name}
									<th scope="col" aria-sort={active ? (list.desc ? 'descending' : 'ascending') : undefined} class:num={f.input === 'number'}>
										<a href={listHref(res.href, list, { sort: active && !list.desc ? `-${f.name}` : f.name })} aria-label={tb('tern.admin.sortBy', { field: f.label })}>
											{f.label}<span class="tern-admin-sort" aria-hidden="true">{active ? (list.desc ? '↓' : '↑') : ''}</span>
										</a>
									</th>
								{/each}
							</tr>
						</thead>
						<tbody>
							{#each list.rows as row (row.key)}
								<tr>
									{#each listed as f, i (f.name)}
										{@const v = row.values[f.name]}
										<td class:num={f.input === 'number'} class:muted={f.input === 'checkbox' && !v}>
											{#if i === 0 || f.name === linkField}
												<a class="tern-admin-open" href="{res.href}/{encodeURIComponent(row.key)}">{shown(f, v, yes, no) || row.key}</a>
											{:else if f.ref && v}
												<a href="{data.base}/{f.ref}/{encodeURIComponent(v)}">{v}</a>
											{:else}
												{shown(f, v, yes, no)}
											{/if}
										</td>
									{/each}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
				<footer class="tern-admin-foot">
					<span>{tb('tern.admin.showing', { from, to: from + list.rows.length - 1, total: list.total })}</span>
					{#if list.pages > 1}
						<Pagination page={list.page} pages={list.pages} href={(p) => listHref(res.href, list, { page: p })} />
					{/if}
				</footer>
			{/if}
		{:else if data.view === 'form' && res && record}
			<p class="tern-admin-back"><a href={res.href}>← {tb('tern.admin.back', { name: lower(res.label) })}</a></p>
			<header class="tern-admin-head">
				<h1>{heading}</h1>
				<div class="tern-admin-buttons">
				{#if editing}
					{#each res.actions ?? [] as act (act.name)}
						<form method="POST" action="?/do" bind:this={actionForms[act.name]} use:enhance>
							<input type="hidden" name="action" value={act.name} />
							<Button
								type="submit"
								variant={act.danger ? 'danger' : 'secondary'}
								onclick={(e: MouseEvent) => {
									if (!act.danger) return;
									e.preventDefault();
									asking = act;
								}}>{act.label}</Button
							>
						</form>
					{/each}
				{/if}
				{#if editing && res.delete}
					<form method="POST" action="?/delete" bind:this={deleteForm} use:enhance>
						<Button
							type="submit"
							variant="danger"
							onclick={(e: MouseEvent) => {
								e.preventDefault();
								confirming = true;
							}}>{tb('tern.admin.delete')}</Button
						>
					</form>
				{/if}
				</div>
			</header>
			{#if notice}<Alert tone="success" live>{notice}</Alert>{/if}
			{#if failure?.message && !failure.errors?.[''] && Object.keys(failure.errors ?? {}).length === 0}
				<Alert tone="danger" live>{failure.message}</Alert>
			{/if}

			<Form action="?/save" rules={editable ? (fields.filter((f) => !f.readonly).map((f) => f.rules) as FormField[]) : []} {form} reset={false} class="tern-admin-form">
				{#each fields as f (f.name)}
					{@const ro = (f.readonly && !(f.name === res.key && !editing)) || !editable}
					{#if ro}
						<div class="tern-admin-static">
							<span class="tern-admin-static-label">{f.label}</span>
							<span class="tern-admin-static-value" class:muted={!shown(f, value(f), yes, no)}>
								{#if f.ref && value(f)}
									<a href="{data.base}/{f.ref}/{encodeURIComponent(value(f))}">{value(f)}</a>
								{:else}
									{shown(f, value(f), yes, no) || '—'}
								{/if}
							</span>
						</div>
					{:else if f.input === 'checkbox'}
						<Checkbox name={f.name} checked={value(f) !== '' && value(f) !== 'false'} description={f.description || undefined}>{f.label}</Checkbox>
					{:else}
						<Field
							label={f.label}
							name={f.name}
							description={f.input === 'password' && editing ? tb('tern.admin.passwordKept') : f.description || undefined}
						>
							{#if f.input === 'textarea'}
								<Textarea name={f.name} value={value(f)} rows={4} />
							{:else if f.input === 'select'}
								<Select name={f.name} value={value(f)} placeholder={f.rules.required ? undefined : '—'} options={[...(f.rules.required ? [] : [{ value: '', label: '—' }]), ...(f.options ?? []).map((o) => ({ value: o, label: o }))]} />
							{:else}
								<Input
									name={f.name}
									type={f.input}
									value={f.input === 'password' ? '' : value(f)}
									step={f.input === 'number' && f.rules.kind === 'float' ? 'any' : undefined}
									autocomplete={f.input === 'password' ? 'new-password' : undefined}
								/>
							{/if}
						</Field>
					{/if}
				{/each}
				{#if editable}
					<div class="tern-admin-actions">
						<Button type="submit">{editing ? tb('tern.admin.save') : tb('tern.admin.create')}</Button>
						<Button href={res.href} variant="ghost">{tb('tern.ui.alertDialog.cancel')}</Button>
					</div>
				{/if}
			</Form>

			{#if editing && res.delete}
				<AlertDialog
					bind:open={confirming}
					title={tb('tern.admin.deleteTitle', { name: lower(res.singular) })}
					description={tb('tern.admin.deleteDescription')}
					confirmLabel={tb('tern.admin.delete')}
					danger
					onconfirm={() => deleteForm?.requestSubmit()}
				/>
			{/if}
			{#if asking}
				<AlertDialog
					open={true}
					title={tb('tern.admin.actionTitle', { label: asking.label })}
					description={tb('tern.admin.deleteDescription')}
					confirmLabel={asking.label}
					danger
					oncancel={() => (asking = null)}
					onconfirm={() => {
						const f = actionForms[asking!.name];
						asking = null;
						f?.requestSubmit();
					}}
				/>
			{/if}
		{/if}
	</main>
</div>

<style>
	.tern-admin {
		display: grid;
		grid-template-columns: 13rem minmax(0, 1fr);
		gap: 2rem;
		align-items: start;
		color: var(--tern-fg);
		font-family: var(--tern-font);
	}
	.tern-admin-nav {
		position: sticky;
		top: 1rem;
		display: grid;
		gap: 0.75rem;
	}
	.tern-admin-title {
		font-weight: 650;
		color: inherit;
		text-decoration: none;
		padding: 0 0.625rem;
	}
	.tern-admin-nav ul {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 2px;
	}
	.tern-admin-nav li a {
		display: block;
		padding: 0.4rem 0.625rem;
		border-radius: var(--tern-radius-sm);
		color: var(--tern-muted);
		text-decoration: none;
		font-size: 0.875rem;
	}
	.tern-admin-nav li a:hover {
		color: var(--tern-fg);
		background: var(--tern-surface);
	}
	.tern-admin-nav li a[aria-current] {
		color: var(--tern-fg);
		background: var(--tern-accent-soft);
		font-weight: 550;
	}
	.tern-admin-main {
		display: grid;
		gap: 1rem;
		min-width: 0;
	}
	.tern-admin-main h1 {
		margin: 0;
		font-size: 1.5rem;
		line-height: 1.25;
	}
	.tern-admin-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
	}
	.tern-admin-buttons {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}
	.tern-admin-head form {
		margin: 0;
	}
	.tern-admin-back {
		margin: 0;
		font-size: 0.875rem;
	}
	.tern-admin-back a {
		color: var(--tern-muted);
		text-decoration: none;
	}
	.tern-admin-back a:hover {
		color: var(--tern-fg);
	}
	.tern-admin-cards {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(12rem, 1fr));
		gap: 0.75rem;
	}
	.tern-admin-cards a {
		display: grid;
		gap: 0.25rem;
		padding: 1rem 1.125rem;
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		background: var(--tern-surface);
		color: inherit;
		text-decoration: none;
		transition: border-color var(--tern-duration);
	}
	.tern-admin-cards a:hover {
		border-color: var(--tern-border-strong);
	}
	.tern-admin-card-label {
		font-weight: 600;
	}
	.tern-admin-card-count {
		color: var(--tern-muted);
		font-size: 0.875rem;
		font-variant-numeric: tabular-nums;
	}
	.tern-admin-query {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		align-items: center;
	}
	.tern-admin-query :global(input[type='search']) {
		flex: 1 1 14rem;
		width: auto;
	}
	.tern-admin-query :global(select) {
		width: auto;
	}
	.tern-admin-scroll {
		overflow-x: auto;
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
	}
	.tern-admin-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.875rem;
	}
	.tern-admin-table th,
	.tern-admin-table td {
		padding: 0.6rem 0.875rem;
		text-align: start;
		border-bottom: 1px solid var(--tern-border);
		white-space: nowrap;
		max-width: 22rem;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.tern-admin-table tbody tr:last-child td {
		border-bottom: 0;
	}
	.tern-admin-table tbody tr:hover {
		background: var(--tern-surface);
	}
	.tern-admin-table th {
		font-weight: 550;
		color: var(--tern-muted);
		background: var(--tern-surface);
	}
	.tern-admin-table th a {
		color: inherit;
		text-decoration: none;
	}
	.tern-admin-table th a:hover,
	.tern-admin-table th[aria-sort] a {
		color: var(--tern-fg);
	}
	.tern-admin-sort {
		display: inline-block;
		width: 1em;
		text-align: center;
	}
	.tern-admin-table .num {
		text-align: end;
		font-variant-numeric: tabular-nums;
	}
	.tern-admin-table .muted {
		color: var(--tern-muted);
	}
	.tern-admin-table td a {
		color: var(--tern-accent);
		text-decoration: none;
	}
	.tern-admin-table td a:hover {
		text-decoration: underline;
	}
	.tern-admin-open {
		font-weight: 550;
	}
	.tern-admin-foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
		color: var(--tern-muted);
		font-size: 0.875rem;
		font-variant-numeric: tabular-nums;
	}
	.tern-admin-static {
		display: grid;
		gap: 0.375rem;
	}
	.tern-admin-static-label {
		font-size: 0.875rem;
		font-weight: 550;
	}
	.tern-admin-static-value {
		font-size: 0.875rem;
		overflow-wrap: anywhere;
	}
	.tern-admin-static-value.muted {
		color: var(--tern-muted);
	}
	.tern-admin-static-value a {
		color: var(--tern-accent);
	}
	:global(.tern-admin-form) {
		display: grid;
		gap: 1rem;
		max-width: 36rem;
	}
	.tern-admin-actions {
		display: flex;
		gap: 0.5rem;
	}
	@media (max-width: 720px) {
		.tern-admin {
			grid-template-columns: minmax(0, 1fr);
			gap: 1rem;
		}
		.tern-admin-nav {
			position: static;
		}
		.tern-admin-nav ul {
			display: flex;
			overflow-x: auto;
			gap: 0.25rem;
		}
		.tern-admin-nav li a {
			white-space: nowrap;
		}
	}
</style>
