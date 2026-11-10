package app

import "testing"

func TestReferrerHost(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.google.com/search?q=tern": "google.com",
		"https://example.com/pricing":          "",
		"https://blog.example.com/post":        "",
		"":                                     "",
		"not a url":                            "",
	} {
		if got := referrerHost(in, "example.com"); got != want {
			t.Errorf("referrerHost(%q) = %q, want %q", in, got, want)
		}
	}
}
