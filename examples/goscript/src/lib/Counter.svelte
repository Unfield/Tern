<script lang="go">
import (
	"strconv"

	"github.com/Unfield/Tern/goscript"
)

func Setup(c *goscript.Component) {
	var props struct {
		Start int `json:"start"`
	}
	_ = c.Props(&props)

	count := goscript.State(c, "count", props.Start)
	step := goscript.State(c, "step", 1)
	goscript.Derived(c, "doubled", func() int { return count.Get() * 2 })
	goscript.Derived(c, "parity", func() string {
		if count.Get()%2 == 0 {
			return "even"
		}
		return "odd"
	})
	history := goscript.State(c, "history", []int{})
	goscript.Effect(c, func() {
		h := append(history.Peek(), count.Get())
		if len(h) > 5 {
			h = h[len(h)-5:]
		}
		history.Set(h)
	})

	c.Func("increment", func() { count.Set(count.Get() + step.Get()) })
	c.Func("reset", func() { count.Set(props.Start) })
	c.Func("setStep", func(e goscript.Event) {
		if n, err := strconv.Atoi(e.Value); err == nil {
			step.Set(n)
		}
	})
}
</script>

<div class="card" data-testid="counter">
	<p>
		<strong data-testid="count">{count}</strong> × 2 = <span data-testid="doubled">{doubled}</span> (<span data-testid="parity">{parity}</span>)
	</p>
	<label>Step <input type="number" value={step} oninput={setStep} data-testid="step" style="width: 5rem" /></label>
	<button onclick={increment}>Add {step}</button>
	<button onclick={reset}>Reset</button>
	<p>Last values: <span data-testid="history">{history?.join(', ')}</span></p>
</div>
