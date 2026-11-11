package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// readDotEnv reads KEY=value lines from a .env file, for tern dev: blank
// lines and # comments are skipped, "export " is allowed, and values may
// be quoted ("…" with escapes, or '…' as written). A missing file is no
// error. Empty values are left out, so .env.example can be copied as it is.
func readDotEnv(path string) ([]string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			return out, fmt.Errorf("line %d: want KEY=value", n)
		}
		value = strings.TrimSpace(value)
		switch {
		case strings.HasPrefix(value, `"`):
			v, err := strconv.Unquote(value)
			if err != nil {
				return out, fmt.Errorf("line %d: %w", n, err)
			}
			value = v
		case strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && len(value) >= 2:
			value = value[1 : len(value)-1]
		default:
			if i := strings.Index(value, " #"); i >= 0 {
				value = strings.TrimSpace(value[:i]) // a comment after the value
			}
		}
		if value == "" {
			continue
		}
		out = append(out, key+"="+value)
	}
	return out, sc.Err()
}
