// Package ua reads a browser, an operating system and a device type from a
// User-Agent header, well enough for analytics.
package ua

import "strings"

// Info is what a User-Agent says about the visitor.
type Info struct {
	Browser, OS, Device string
	// Bot is true for crawlers and monitors, which aren't counted.
	Bot bool
}

var bots = []string{"bot", "crawl", "spider", "slurp", "facebookexternalhit", "monitor", "curl/", "wget/", "python-requests", "go-http-client", "headless-shell", "lighthouse"}

// Parse reads a User-Agent header.
func Parse(s string) Info {
	l := strings.ToLower(s)
	var in Info
	if s == "" {
		in.Bot = true
	}
	for _, b := range bots {
		if strings.Contains(l, b) {
			in.Bot = true
		}
	}
	switch {
	case strings.Contains(l, "edg/"):
		in.Browser = "Edge"
	case strings.Contains(l, "opr/") || strings.Contains(l, "opera"):
		in.Browser = "Opera"
	case strings.Contains(l, "samsungbrowser"):
		in.Browser = "Samsung Internet"
	case strings.Contains(l, "firefox/") || strings.Contains(l, "fxios"):
		in.Browser = "Firefox"
	case strings.Contains(l, "chrome/") || strings.Contains(l, "crios"):
		in.Browser = "Chrome"
	case strings.Contains(l, "safari/"):
		in.Browser = "Safari"
	default:
		in.Browser = "Other"
	}
	switch {
	case strings.Contains(l, "iphone") || strings.Contains(l, "ipad") || strings.Contains(l, "ipod"):
		in.OS = "iOS"
	case strings.Contains(l, "android"):
		in.OS = "Android"
	case strings.Contains(l, "windows"):
		in.OS = "Windows"
	case strings.Contains(l, "mac os x") || strings.Contains(l, "macintosh"):
		in.OS = "macOS"
	case strings.Contains(l, "cros"):
		in.OS = "ChromeOS"
	case strings.Contains(l, "linux"):
		in.OS = "Linux"
	default:
		in.OS = "Other"
	}
	switch {
	case strings.Contains(l, "ipad") || (strings.Contains(l, "android") && !strings.Contains(l, "mobile")) || strings.Contains(l, "tablet"):
		in.Device = "Tablet"
	case strings.Contains(l, "mobi") || strings.Contains(l, "iphone"):
		in.Device = "Phone"
	default:
		in.Device = "Desktop"
	}
	return in
}
