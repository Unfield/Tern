<script lang="go" live>
import "github.com/Unfield/Tern/goscript"

// Shared by everyone: this code runs on the server, so package variables
// are one value for all visitors.
var (
	clicks = goscript.Global(0)
	online = goscript.Global(0)
)

func Setup(c *goscript.Component) {
	var props struct {
		Name string `json:"name"`
	}
	_ = c.Props(&props)

	clicks.Show(c, "clicks")
	online.Show(c, "online")
	mine := goscript.State(c, "mine", 0)
	goscript.Derived(c, "share", func() string {
		if clicks.Get() == 0 {
			return "no clicks yet"
		}
		return fmtPercent(mine.Get(), clicks.Get()) + " of all clicks are yours"
	})
	goscript.State(c, "hello", "Hello, "+props.Name)

	// Count this visitor while the page is open, not while it renders.
	if !c.Rendering() {
		online.Update(func(n int) int { return n + 1 })
		c.OnDestroy(func() { online.Update(func(n int) int { return n - 1 }) })
	}

	c.Func("click", func() {
		clicks.Update(func(n int) int { return n + 1 })
		mine.Update(func(n int) int { return n + 1 })
	})
}

func fmtPercent(a, b int) string {
	return itoa(a*100/b) + "%"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
</script>

<div class="card" data-testid="live">
	<h3>Live: Go on the server</h3>
	<p data-testid="hello">{hello}</p>
	<p><strong data-testid="online">{online}</strong> online · <strong data-testid="clicks">{clicks}</strong> clicks in total</p>
	<button onclick={click}>Click</button>
	<p data-testid="share">{share}</p>
</div>
