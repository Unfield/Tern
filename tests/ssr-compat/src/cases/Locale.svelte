<script lang="ts">
	const n = 5_000_000;
	const price = 1234.5;
</script>

<p>{n.toLocaleString()} {n.toLocaleString('en-US')} {price.toLocaleString('en-US', { style: 'currency', currency: 'USD' })} {(0.256).toLocaleString('en-US', { style: 'percent' })}</p>
