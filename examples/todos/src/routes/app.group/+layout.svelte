<script lang="ts">
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
</script>

<header>
	<a href="/">Todos</a>
	<span>
		Signed in as <strong data-testid="user">{data.signedInAs}</strong>
		<form method="POST" action="/logout">
			<button class="link">Sign out</button>
		</form>
	</span>
</header>

{@render children()}

<style>
	header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem 1.25rem;
		background: white;
		border-bottom: 1px solid #e7e5e4;
	}
	header form {
		display: inline;
	}
	.link {
		background: none;
		border: none;
		color: #0f766e;
		text-decoration: underline;
		padding: 0 0 0 0.5rem;
	}
</style>
