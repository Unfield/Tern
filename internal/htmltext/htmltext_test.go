package htmltext

import "testing"

func TestConvert(t *testing.T) {
	in := `<!doctype html><html><head><title>Hi</title><style>p{color:red}</style></head><body>
<h1>Welcome,   Ada!</h1>
<p>Thanks for signing up. <a href="https://example.com/confirm?t=1">Confirm your email</a> to start.</p>
<ul><li>One</li><li>Two &amp; three</li></ul>
<p>Line<br>break</p><a href="https://example.com">https://example.com</a>
<pre>  code
  block</pre>
</body></html>`
	want := "Welcome, Ada!\n\nThanks for signing up. Confirm your email (https://example.com/confirm?t=1) to start.\n\n- One\n- Two & three\n\nLine\nbreak\n\nhttps://example.com\n\n  code\n  block"
	if got := Convert(in); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}
