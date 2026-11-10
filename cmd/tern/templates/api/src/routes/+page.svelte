<script lang="ts">
	import { ApiError } from '@ternkit/tern';
	import { api } from '$api';
	import { onMount } from 'svelte';

	type Todo = Awaited<ReturnType<typeof api<'GET /api/todos/[id]'>>>;
	let todos = $state<Todo[]>([]);
	let title = $state('');
	let error = $state('');

	async function refresh() {
		todos = (await api('GET /api/todos')) ?? [];
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		try {
			await api('POST /api/todos', { body: { title } });
			title = '';
			await refresh();
		} catch (err) {
			error = err instanceof ApiError ? err.message : String(err);
		}
	}

	onMount(refresh);
</script>

<svelte:head>
	<title>API</title>
</svelte:head>

<h1>Todos</h1>
<p>This page calls the typed endpoints in <code>src/routes/api</code> with <code>$api</code>. Their OpenAPI description is at <a href="/api/openapi.json">/api/openapi.json</a>.</p>

<form onsubmit={add}>
	<input bind:value={title} placeholder="Something to do" aria-label="New todo" />
	<button>Add</button>
</form>
{#if error}<p class="error" role="alert">{error}</p>{/if}

<ul data-testid="todos">
	{#each todos as todo (todo.id)}
		<li>
			<label>
				<input
					type="checkbox"
					checked={todo.done}
					onchange={async (e) => {
						await api('PATCH /api/todos/[id]', { params: { id: todo.id }, body: { done: e.currentTarget.checked } });
						await refresh();
					}}
				/>
				{todo.title}
			</label>
			<button
				onclick={async () => {
					await api('DELETE /api/todos/[id]', { params: { id: todo.id } });
					await refresh();
				}}>Delete</button
			>
		</li>
	{/each}
</ul>

<style>
	form {
		display: flex;
		gap: 0.5rem;
	}
	input,
	button {
		font: inherit;
		padding: 0.4rem 0.75rem;
	}
	.error {
		color: #b91c1c;
	}
</style>
