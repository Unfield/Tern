<script lang="ts">
	type Item = { id: number; name: string; price: number; tags: string[]; updated: string; stock: number };
	let { items }: { items: Item[] } = $props();
	const total = $derived(items.reduce((sum, i) => sum + i.price * i.stock, 0));
	const out = $derived(items.filter((i) => i.stock === 0).length);
</script>

<p>{items.length} items, {out} out of stock, worth {total.toFixed(2)} in all.</p>
<table>
	<thead>
		<tr><th>#</th><th>Name</th><th>Tags</th><th>Price</th><th>Stock</th><th>Updated</th></tr>
	</thead>
	<tbody>
		{#each items as item (item.id)}
			<tr class:out={item.stock === 0}>
				<td>{item.id}</td>
				<td><a href="/items/{item.id}">{item.name}</a></td>
				<td>{#each item.tags as tag (tag)}<span class="tag">{tag}</span>{/each}</td>
				<td>{item.price.toFixed(2)}</td>
				<td>{item.stock === 0 ? 'Out of stock' : item.stock}</td>
				<td>{item.updated.slice(0, 10)}</td>
			</tr>
		{/each}
	</tbody>
</table>
