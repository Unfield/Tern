<script lang="ts">
	import { Debounced, PersistedState, useDebounce, ElementSize } from 'runed';
	let query = $state('svelte');
	const debounced = new Debounced(() => query, 300);
	const persisted = new PersistedState('count', 3);
	const save = useDebounce(() => {}, 100);
	save();
</script>

<p>debounced: {debounced.current} persisted: {persisted.current}</p>
