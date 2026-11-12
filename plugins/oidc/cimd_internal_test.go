package oidc

import (
	"testing"
	"time"
)

func TestPublicOnly(t *testing.T) {
	for addr, ok := range map[string]bool{
		"93.184.215.14:443":           true,
		"[2606:2800:21f:cb07::1]:443": true,
		"93.184.215.14:80":            false,
		"127.0.0.1:443":               false,
		"10.0.0.5:443":                false,
		"192.168.1.1:443":             false,
		"172.16.0.1:443":              false,
		"169.254.169.254:443":         false, // cloud metadata
		"100.64.0.1:443":              false,
		"0.0.0.0:443":                 false,
		"[::1]:443":                   false,
		"[fd00::1]:443":               false,
		"[fe80::1]:443":               false,
		"[::ffff:127.0.0.1]:443":      false,
		"[64:ff9b::a00:1]:443":        false,
	} {
		if err := publicOnly("tcp", addr, nil); (err == nil) != ok {
			t.Errorf("%s: %v", addr, err)
		}
	}
}

func TestMetadataURLsAndRedirects(t *testing.T) {
	for id, ok := range map[string]bool{
		"https://claude.ai/oauth/client.json": true,
		"https://claude.ai":                   false,
		"https://claude.ai/":                  false,
		"http://claude.ai/client.json":        false,
		"https://claude.ai/c.json?x=1":        false,
		"https://user@claude.ai/c.json":       false,
		"https://claude.ai/a/../c.json":       false,
		"spa":                                 false,
	} {
		if isMetadataURL(id) != ok {
			t.Errorf("isMetadataURL(%q) = %v", id, !ok)
		}
	}
	for r, ok := range map[string]bool{
		"https://claude.ai/callback": true,
		"http://localhost:3334/cb":   true,
		"http://127.0.0.1/cb":        true,
		"http://[::1]:9/cb":          true,
		"http://claude.ai/callback":  false,
		"com.example.app:/cb":        false,
		"https://claude.ai/cb#x":     false,
		"https://evil@claude.ai/cb":  false,
	} {
		if redirectAllowed(r) != ok {
			t.Errorf("redirectAllowed(%q) = %v", r, !ok)
		}
	}
	for cc, want := range map[string]time.Duration{
		"":                     time.Hour,
		"max-age=60":           5 * time.Minute,
		"public, max-age=7200": 2 * time.Hour,
		"max-age=999999":       24 * time.Hour,
		"no-store":             5 * time.Minute,
	} {
		if got := cacheTTL(cc); got != want {
			t.Errorf("cacheTTL(%q) = %s, want %s", cc, got, want)
		}
	}
}
