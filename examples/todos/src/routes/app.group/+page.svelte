<script lang="ts">
	import { enhance } from '@unfield/tern';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
</script>

<svelte:head>
	<title>Todos</title>
</svelte:head>

<main>
	<h1>Your todos</h1>

	<form method="POST" action="?/create" use:enhance class="new">
		<input name="title" placeholder="What needs doing?" aria-label="New todo" value={form?.title ?? ''} />
		<button>Add</button>
	</form>
	{#if form?.error}<p class="error" role="alert">{form.error}</p>{/if}

	{#if data.todos?.length}
		<ul>
			{#each data.todos as todo (todo.id)}
				<li class:done={todo.done}>
					<form method="POST" action="?/toggle" use:enhance>
						<input type="hidden" name="id" value={todo.id} />
						<button class="check" aria-label={todo.done ? 'Mark as not done' : 'Mark as done'}>{todo.done ? '✓' : ''}</button>
					</form>
					<a href="/todos/{todo.id}">{todo.title}</a>
					<form method="POST" action="?/delete" use:enhance>
						<input type="hidden" name="id" value={todo.id} />
						<button class="delete" aria-label="Delete {todo.title}">×</button>
					</form>
				</li>
			{/each}
		</ul>
		<p data-testid="left">{data.left} left</p>
	{:else}
		<p>Nothing to do. Add your first todo above.</p>
	{/if}
</main>

<style>
	.new {
		display: flex;
		gap: 0.5rem;
	}
	.new input {
		flex: 1;
	}
	ul {
		list-style: none;
		padding: 0;
	}
	li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.5rem 0;
		border-bottom: 1px solid #e7e5e4;
	}
	li a {
		flex: 1;
		color: inherit;
		text-decoration: none;
	}
	li.done a {
		text-decoration: line-through;
		color: #a8a29e;
	}
	.check {
		width: 1.75rem;
		height: 1.75rem;
		padding: 0;
		background: white;
		color: #0f766e;
	}
	.delete {
		background: none;
		border: none;
		color: #a8a29e;
		font-size: 1.25rem;
		padding: 0 0.25rem;
	}
</style>
