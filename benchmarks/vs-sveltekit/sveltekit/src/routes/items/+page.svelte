<script lang="ts">
	import Shell from '../../../../shared/Shell.svelte';
	import Items from '../../../../shared/Items.svelte';
	let { data } = $props();
</script>

<Shell title="Items"><Items items={data.items} /></Shell>
