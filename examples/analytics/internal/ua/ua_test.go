package ua

import "testing"

func TestParse(t *testing.T) {
	for s, want := range map[string]Info{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15":          {"Safari", "macOS", "Desktop", false},
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0":  {"Edge", "Windows", "Desktop", false},
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1": {"Safari", "iOS", "Phone", false},
		"Mozilla/5.0 (Linux; Android 14; SM-X710) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/26.0 Chrome/122.0.0.0 Safari/537.36": {"Samsung Internet", "Android", "Tablet", false},
		"Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0":                                                          {"Firefox", "Linux", "Desktop", false},
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)":                                                       {"Other", "Other", "Desktop", true},
		"curl/8.7.1": {"Other", "Other", "Desktop", true},
	} {
		if got := Parse(s); got != want {
			t.Errorf("Parse(%q) = %+v, want %+v", s, got, want)
		}
	}
}
