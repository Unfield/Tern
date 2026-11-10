<script lang="ts">
	import Shell from '../../../shared/Shell.svelte';
	import Home from '../../../shared/Home.svelte';
	let { data } = $props();
</script>

<Shell title="Home"><Home greeting={data.greeting} /></Shell>
