<script lang="go">
import (
	"time"

	"github.com/Unfield/Tern/goscript"
)

func Setup(c *goscript.Component) {
	limit := goscript.State(c, "limit", 5_000_000)
	result := goscript.State(c, "result", 0)
	ms := goscript.State(c, "ms", 0.0)

	c.Func("run", func() {
		start := time.Now()
		result.Set(countPrimes(limit.Get()))
		ms.Set(float64(time.Since(start).Microseconds()) / 1000)
	})
}

// countPrimes counts the primes up to n with the sieve of Eratosthenes.
func countPrimes(n int) int {
	composite := make([]bool, n+1)
	count := 0
	for i := 2; i <= n; i++ {
		if composite[i] {
			continue
		}
		count++
		for j := i * i; j <= n; j += i {
			composite[j] = true
		}
	}
	return count
}
</script>

<div class="card" data-testid="primes-go">
	<h3>Go (WebAssembly)</h3>
	<button onclick={run}>Count primes up to {limit.toLocaleString('en-US')}</button>
	<p>Result: <strong data-testid="result">{result}</strong> in <span data-testid="ms">{ms}</span> ms</p>
</div>
