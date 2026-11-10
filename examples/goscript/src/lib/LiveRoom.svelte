<script lang="go" live>
import (
	"strconv"

	"github.com/Unfield/Tern/goscript"
)

// Shared by everyone: this code runs on the server, so package variables
// are one value for all visitors. Presence counts across server processes
// too, when the app has a broker.
var (
	clicks = goscript.Global(0)
	online = goscript.Presence("room")
)

func Setup(c *goscript.Component) {
	var props struct {
		Name string `json:"name"`
	}
	_ = c.Props(&props)

	total := clicks.Show(c, "clicks")
	online.Show(c, "online")
	mine := goscript.State(c, "mine", 0)
	goscript.Derived(c, "share", func() string {
		if total.Get() == 0 {
			return "no clicks yet"
		}
		return fmtPercent(mine.Get(), total.Get()) + " of all clicks are yours"
	})
	goscript.State(c, "hello", "Hello, "+props.Name)

	// Count this visitor while the page is open (not while it renders).
	online.Join(c)

	c.Func("click", func() {
		clicks.Update(func(n int) int { return n + 1 })
		mine.Update(func(n int) int { return n + 1 })
	})
}

func fmtPercent(a, b int) string {
	return strconv.Itoa(a*100/b) + "%"
}
</script>

<div class="card" data-testid="live">
	<h3>Live: Go on the server</h3>
	<p data-testid="hello">{hello}</p>
	<p><strong data-testid="online">{online}</strong> online · <strong data-testid="clicks">{clicks}</strong> clicks in total</p>
	<button onclick={click}>Click</button>
	<p data-testid="share">{share}</p>
</div>
