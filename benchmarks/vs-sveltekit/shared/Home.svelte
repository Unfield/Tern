<script lang="ts">
	let { greeting }: { greeting: string } = $props();
	const features = Array.from({ length: 10 }, (_, i) => ({ id: i, name: `Feature ${i + 1}`, text: `What feature ${i + 1} does, in one sentence.` }));
</script>

<p class="lead">{greeting}</p>
<ul class="features">
	{#each features as f (f.id)}
		<li>
			<h2>{f.name}</h2>
			<p>{f.text}</p>
		</li>
	{/each}
</ul>
