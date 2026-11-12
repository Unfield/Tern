package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type config struct {
	DatabaseURL string         `env:"DATABASE_URL,required"`
	Secret      Secret         `env:"SESSION_SECRET,required,min=32"`
	Workers     int            `env:"WORKERS" default:"4"`
	Timeout     time.Duration  `env:"TIMEOUT" default:"10s"`
	Debug       bool           `env:"DEBUG"`
	Admins      []int64        `env:"ADMINS"`
	Listen      netip.AddrPort `env:"LISTEN" default:"127.0.0.1:3000"`
	Mail        struct {
		Host string `env:"SMTP_HOST" default:"localhost"`
	}
	Untouched string
}

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestLoad(t *testing.T) {
	var c config
	c.Untouched = "kept"
	err := LoadFrom(&c, env(map[string]string{
		"DATABASE_URL":   "postgres://db",
		"SESSION_SECRET": strings.Repeat("s", 32),
		"WORKERS":        "8",
		"ADMINS":         "1, 2,",
		"DEBUG":          "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.DatabaseURL != "postgres://db" || c.Secret.Reveal() != strings.Repeat("s", 32) || c.Workers != 8 || c.Timeout != 10*time.Second ||
		!c.Debug || len(c.Admins) != 2 || c.Admins[1] != 2 || c.Listen.Port() != 3000 || c.Mail.Host != "localhost" || c.Untouched != "kept" {
		t.Errorf("%+v", c)
	}
}

func TestLoadReportsEverything(t *testing.T) {
	var c config
	err := LoadFrom(&c, env(map[string]string{"SESSION_SECRET": "short", "WORKERS": "many", "TIMEOUT": "soon"}))
	var e *Error
	if !errors.As(err, &e) || len(e.Problems) != 4 {
		t.Fatalf("%v", err)
	}
	for _, want := range []string{"DATABASE_URL is required", "SESSION_SECRET must be at least 32 characters, and is 5", `WORKERS: "many" isn't a whole number`, `TIMEOUT: "soon" isn't a duration`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%v lacks %q", err, want)
		}
	}
	// The secret's value isn't in the error either.
	if strings.Contains(err.Error(), "short") {
		t.Errorf("the error shows the secret: %v", err)
	}
}

func TestSecretStaysSecret(t *testing.T) {
	c := config{Secret: "hunter2-hunter2-hunter2-hunter2-x"}
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("starting", "settings", c, "secret", c.Secret)
	j, _ := json.Marshal(c)
	for what, out := range map[string]string{
		"%v": fmt.Sprintf("%v", c), "%+v": fmt.Sprintf("%+v", c), "%#v": fmt.Sprintf("%#v", c), "%s": c.Secret.String(),
		"json": string(j), "slog": logs.String(),
	} {
		if strings.Contains(out, "hunter2") {
			t.Errorf("%s shows the secret: %s", what, out)
		}
	}
	if Secret("").String() != "" {
		t.Error("an empty secret should print empty, to tell it's unset")
	}
}
