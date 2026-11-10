<script lang="ts">
	import { Area, Axis, Chart, Svg } from 'layerchart';
	const data = Array.from({ length: 10 }, (_, i) => ({ x: i, y: i * i }));
</script>

<div style="height: 200px; width: 400px">
	<Chart {data} x="x" y="y" yDomain={[0, null]} width={400} height={200}>
		<Svg><Axis placement="left" /><Area class="area" /></Svg>
	</Chart>
</div>
