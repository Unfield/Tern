<script lang="ts" generics="Row extends object">
	import type { ComponentProps } from 'svelte';
	import LineChart from './LineChart.svelte';

	let props: Omit<ComponentProps<typeof LineChart<Row>>, 'area'> = $props();
</script>

<!-- A LineChart with each series' area washed in under its line. -->
<LineChart {...props} area />
