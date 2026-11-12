package kit

import (
	"strings"
	"testing"

	"github.com/Unfield/Tern/native/js"
)

// A binary built without the JavaScript engine has the compiled renderer
// and no server bundle: pages must still render on the server, with the
// nonce on the inline scripts of their head.
func TestNativeWithoutBundle(t *testing.T) {
	m := testManifest()
	m.Native = func(setup func(*js.Realm)) *js.Realm {
		rt := js.NewRealm()
		setup(rt)
		rt.Global.DefineHidden("__tern_render", rt.NewFunction("__tern_render", 2, func(js.Value, []js.Value) js.Value {
			return `{"head":"<script>theme()</script>","body":"<p>native</p>"}`
		}))
		return rt
	}
	a, _ := newTestApp(t, m)
	body := do(t, a, "GET", "/", nil).Body.String()
	if !strings.Contains(body, "<p>native</p>") {
		t.Fatalf("not rendered on the server:\n%s", body)
	}
	if !strings.Contains(body, `<script nonce="`) || strings.Contains(body, "<script>theme()") {
		t.Errorf("the head's inline script has no nonce:\n%s", body)
	}
}
