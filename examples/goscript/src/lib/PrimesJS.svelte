<script lang="ts">
	const limit = 5_000_000;
	let result = $state(0);
	let ms = $state(0);

	function countPrimes(n: number) {
		const composite = new Uint8Array(n + 1);
		let count = 0;
		for (let i = 2; i <= n; i++) {
			if (composite[i]) continue;
			count++;
			for (let j = i * i; j <= n; j += i) composite[j] = 1;
		}
		return count;
	}

	function run() {
		const start = performance.now();
		result = countPrimes(limit);
		ms = Math.round((performance.now() - start) * 1000) / 1000;
	}
</script>

<div class="card" data-testid="primes-js">
	<h3>TypeScript</h3>
	<button onclick={run}>Count primes up to {limit.toLocaleString('en-US')}</button>
	<p>Result: <strong data-testid="result">{result}</strong> in <span data-testid="ms">{ms}</span> ms</p>
</div>
