package codegen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// ConfigFile is the optional project configuration, shared by the code
// generator, the Vite plugin and the runtime.
const ConfigFile = "tern.json"

// Config is the contents of tern.json.
type Config struct {
	// BasePath serves the app under a URL prefix such as "/app". It must
	// start with a slash and must not end with one. The default is "".
	BasePath string `json:"basePath"`
	// I18n configures translations in src/locales, if the app has them.
	I18n struct {
		// DefaultLocale is used when no other locale fits, and for messages
		// missing from a locale. It defaults to "en", or the first locale.
		DefaultLocale string `json:"defaultLocale"`
	} `json:"i18n"`
	// ServiceWorker configures src/service-worker.ts, if the app has one.
	ServiceWorker struct {
		// Register is whether the client registers the service worker. It
		// defaults to true; set it to false to register it yourself.
		Register *bool `json:"register"`
	} `json:"serviceWorker"`
	// Native is whether tern build compiles the server renderer to Go, so
	// pages render without the JavaScript engine (ADR 0011). It defaults
	// to true.
	Native *bool `json:"native"`
}

// NativeEnabled reports whether tern build compiles the server renderer
// to Go.
func (c Config) NativeEnabled() bool { return c.Native == nil || *c.Native }

var basePathRe = regexp.MustCompile(`^(/[A-Za-z0-9._~!$&'()*+,;=:@-]+)+$`)

// LoadConfig reads tern.json from root. A missing file is an empty config.
func LoadConfig(root string) (Config, error) {
	var c Config
	b, err := os.ReadFile(filepath.Join(root, ConfigFile))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", ConfigFile, err)
	}
	if c.BasePath != "" && !basePathRe.MatchString(c.BasePath) {
		return c, fmt.Errorf("%s: basePath %q must start with / and must not end with one, such as \"/app\"", ConfigFile, c.BasePath)
	}
	return c, nil
}
