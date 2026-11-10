<script lang="ts">
	import LiveMonitors from '$lib/LiveMonitors.svelte';
</script>

<svelte:head><title>Monitors · Status</title></svelte:head>

<!-- The dashboard is a live Go component: it loads the monitors and follows
     every check on the server, and the page shows what it publishes. -->
<LiveMonitors />
