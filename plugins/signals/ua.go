package signals

import (
	"strconv"
	"strings"
)

// Client is what a request's User-Agent says it is.
type Client struct {
	// Browser is Chrome, Edge, Opera, Samsung Internet, Firefox or Safari,
	// or "" for anything else.
	Browser string `json:"browser,omitempty"`
	// Version is the browser's major version.
	Version int `json:"version,omitempty"`
	// OS is Windows, macOS, iOS, Android, Linux or ChromeOS, or "".
	OS     string `json:"os,omitempty"`
	Mobile bool   `json:"mobile,omitempty"`
	// Crawler names a search engine or other crawler that says it is one,
	// such as Googlebot. It says nothing of whether it's true.
	Crawler string `json:"crawler,omitempty"`
	// Tool names an HTTP library or automation tool, such as curl,
	// python-requests or HeadlessChrome.
	Tool string `json:"tool,omitempty"`
}

// Chromium reports whether the browser is built on Chromium.
func (c Client) Chromium() bool {
	switch c.Browser {
	case "Chrome", "Edge", "Opera", "Samsung Internet":
		return true
	}
	return false
}

// tools are user agents of HTTP libraries and automation, matched as
// prefixes or substrings (lowercase).
var tools = []struct{ match, name string }{
	{"headlesschrome", "HeadlessChrome"},
	{"phantomjs", "PhantomJS"},
	{"curl/", "curl"},
	{"wget/", "Wget"},
	{"python-requests", "python-requests"},
	{"python-urllib", "python-urllib"},
	{"python-httpx", "python-httpx"},
	{"aiohttp", "aiohttp"},
	{"httpie", "HTTPie"},
	{"go-http-client", "Go-http-client"},
	{"okhttp", "okhttp"},
	{"java/", "Java"},
	{"apache-httpclient", "Apache-HttpClient"},
	{"axios/", "axios"},
	{"node-fetch", "node-fetch"},
	{"undici", "undici"},
	{"got (", "got"},
	{"postmanruntime", "PostmanRuntime"},
	{"insomnia", "Insomnia"},
	{"scrapy", "Scrapy"},
	{"libwww-perl", "libwww-perl"},
	{"guzzlehttp", "Guzzle"},
	{"ruby", "Ruby"},
	{"dart:io", "Dart"},
	{"reqwest", "reqwest"},
}

var crawlers = []struct{ match, name string }{
	{"googlebot", "Googlebot"},
	{"google-inspectiontool", "Google-InspectionTool"},
	{"adsbot-google", "AdsBot-Google"},
	{"bingbot", "Bingbot"},
	{"duckduckbot", "DuckDuckBot"},
	{"yandexbot", "YandexBot"},
	{"baiduspider", "Baiduspider"},
	{"applebot", "Applebot"},
	{"facebookexternalhit", "facebookexternalhit"},
	{"twitterbot", "Twitterbot"},
	{"linkedinbot", "LinkedInBot"},
	{"slackbot", "Slackbot"},
	{"discordbot", "Discordbot"},
	{"telegrambot", "TelegramBot"},
	{"whatsapp", "WhatsApp"},
	{"gptbot", "GPTBot"},
	{"chatgpt-user", "ChatGPT-User"},
	{"claudebot", "ClaudeBot"},
	{"claude-user", "Claude-User"},
	{"perplexitybot", "PerplexityBot"},
	{"ccbot", "CCBot"},
	{"bytespider", "Bytespider"},
	{"ahrefsbot", "AhrefsBot"},
	{"semrushbot", "SemrushBot"},
	{"petalbot", "PetalBot"},
	{"mj12bot", "MJ12bot"},
	{"dotbot", "DotBot"},
	{"uptimerobot", "UptimeRobot"},
}

// parseUA reads a User-Agent.
func parseUA(ua string) Client {
	var c Client
	low := strings.ToLower(ua)
	for _, t := range crawlers {
		if strings.Contains(low, t.match) {
			c.Crawler = t.name
			break
		}
	}
	if c.Crawler == "" && (strings.Contains(low, "bot/") || strings.Contains(low, "crawler") || strings.Contains(low, "spider")) {
		c.Crawler = "other"
	}
	for _, t := range tools {
		if strings.Contains(low, t.match) {
			c.Tool = t.name
			break
		}
	}

	switch {
	case strings.Contains(ua, "Windows"):
		c.OS = "Windows"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"), strings.Contains(ua, "iPod"):
		c.OS = "iOS"
	case strings.Contains(ua, "Android"):
		c.OS = "Android"
	case strings.Contains(ua, "CrOS"):
		c.OS = "ChromeOS"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		c.OS = "macOS"
	case strings.Contains(ua, "Linux"):
		c.OS = "Linux"
	}
	c.Mobile = strings.Contains(ua, "Mobile") || c.OS == "iOS" && !strings.Contains(ua, "iPad")

	if c.Tool != "" || !strings.HasPrefix(ua, "Mozilla/5.0") {
		return c
	}
	switch {
	case has(ua, "Edg/"), has(ua, "EdgA/"), has(ua, "EdgiOS/"):
		c.Browser, c.Version = "Edge", major(ua, "Edg/", "EdgA/", "EdgiOS/")
	case has(ua, "OPR/"):
		c.Browser, c.Version = "Opera", major(ua, "OPR/")
	case has(ua, "SamsungBrowser/"):
		c.Browser, c.Version = "Samsung Internet", major(ua, "SamsungBrowser/")
	case has(ua, "Firefox/"), has(ua, "FxiOS/"):
		c.Browser, c.Version = "Firefox", major(ua, "Firefox/", "FxiOS/")
	case has(ua, "Chrome/"), has(ua, "CriOS/"):
		c.Browser, c.Version = "Chrome", major(ua, "Chrome/", "CriOS/")
	case has(ua, "Safari/") && has(ua, "Version/"):
		c.Browser, c.Version = "Safari", major(ua, "Version/")
	}
	return c
}

func has(s, sub string) bool { return strings.Contains(s, sub) }

// major is the number after the first of the markers found.
func major(ua string, markers ...string) int {
	for _, m := range markers {
		i := strings.Index(ua, m)
		if i < 0 {
			continue
		}
		rest := ua[i+len(m):]
		end := 0
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		n, _ := strconv.Atoi(rest[:end])
		return n
	}
	return 0
}
