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
}

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
