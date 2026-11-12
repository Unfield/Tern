// Package settings reads an app's settings from the environment into a
// struct, declared once and checked when the app starts:
//
//	type Settings struct {
//		DatabaseURL   string           `env:"DATABASE_URL,required"`
//		SessionSecret settings.Secret  `env:"SESSION_SECRET,required,min=32"`
//		Workers       int              `env:"WORKERS" default:"4"`
//		Timeout       time.Duration    `env:"TIMEOUT" default:"10s"`
//		Admins        []string         `env:"ADMINS"`
//	}
//
//	var s Settings
//	if err := settings.Load(&s); err != nil {
//		log.Fatal(err) // every missing or invalid setting, at once
//	}
//
// A Secret never shows its value: printed, logged with slog or encoded as
// JSON it's "[secret]", so a log line or an error page can't leak it.
// Reveal returns it, where it's used.
package settings

import (
	"encoding"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Secret is a setting that must not be shown: a password, an API key, a
// signing secret.
type Secret string

const hidden = "[secret]"

// Reveal returns the secret's value.
func (s Secret) Reveal() string { return string(s) }

// String implements fmt.Stringer, without the value.
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return hidden
}

// GoString implements fmt.GoStringer, without the value.
func (s Secret) GoString() string { return `settings.Secret("` + s.String() + `")` }

// LogValue implements slog.LogValuer, without the value.
func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

// MarshalJSON implements json.Marshaler, without the value.
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(strconv.Quote(s.String())), nil }

// MarshalText implements encoding.TextMarshaler, without the value.
func (s Secret) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Error lists every setting that's missing or invalid.
type Error struct {
	Problems []string
}

func (e *Error) Error() string {
	return "settings: " + strings.Join(e.Problems, "; ")
}

// Load fills the struct v points to from the environment, by the fields'
// env tags: `env:"NAME"`, with ",required" for a setting the app can't run
// without and ",min=N" for a shortest length, and an optional
// `default:"…"`. Fields may be strings, Secrets, bools, integers, floats,
// time.Durations, slices of those (comma-separated), or types that
// implement encoding.TextUnmarshaler. Fields without an env tag are left
// alone; nested structs are read too.
func Load(v any) error { return LoadFrom(v, os.LookupEnv) }

// LoadFrom is Load with another source of values, such as a map in tests.
func LoadFrom(v any, lookup func(string) (string, bool)) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return errors.New("settings: Load takes a pointer to a struct")
	}
	e := &Error{}
	load(rv.Elem(), lookup, e)
	if len(e.Problems) > 0 {
		return e
	}
	return nil
}

func load(v reflect.Value, lookup func(string) (string, bool), e *Error) {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag, ok := f.Tag.Lookup("env")
		if !ok {
			if f.Type.Kind() == reflect.Struct && f.Type != reflect.TypeFor[time.Time]() {
				load(v.Field(i), lookup, e)
			}
			continue
		}
		parts := strings.Split(tag, ",")
		name := parts[0]
		required, minLen := false, 0
		for _, opt := range parts[1:] {
			switch {
			case opt == "required":
				required = true
			case strings.HasPrefix(opt, "min="):
				minLen, _ = strconv.Atoi(strings.TrimPrefix(opt, "min="))
			}
		}
		raw, set := lookup(name)
		if !set || raw == "" {
			if def, ok := f.Tag.Lookup("default"); ok {
				raw, set = def, true
			}
		}
		if !set || raw == "" {
			if required {
				e.Problems = append(e.Problems, name+" is required and isn't set")
			}
			continue
		}
		if minLen > 0 && len(raw) < minLen {
			e.Problems = append(e.Problems, fmt.Sprintf("%s must be at least %d characters, and is %d", name, minLen, len(raw)))
			continue
		}
		if err := set1(v.Field(i), raw); err != nil {
			e.Problems = append(e.Problems, fmt.Sprintf("%s: %v", name, err))
		}
	}
}

var durationType = reflect.TypeFor[time.Duration]()

func set1(f reflect.Value, raw string) error {
	if u, ok := f.Addr().Interface().(encoding.TextUnmarshaler); ok && f.Type() != reflect.TypeFor[Secret]() {
		return u.UnmarshalText([]byte(raw))
	}
	switch {
	case f.Type() == durationType:
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%q isn't a duration, such as 30s or 5m", raw)
		}
		f.SetInt(int64(d))
		return nil
	case f.Kind() == reflect.Slice && f.Type().Elem().Kind() != reflect.Uint8:
		parts := strings.Split(raw, ",")
		s := reflect.MakeSlice(f.Type(), 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p == "" {
				continue
			}
			el := reflect.New(f.Type().Elem()).Elem()
			if err := set1(el, p); err != nil {
				return err
			}
			s = reflect.Append(s, el)
		}
		f.Set(s)
		return nil
	}
	switch f.Kind() {
	case reflect.String:
		f.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("%q isn't true or false", raw)
		}
		f.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, f.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q isn't a whole number", raw)
		}
		f.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, f.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q isn't a whole number of 0 or more", raw)
		}
		f.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(raw, f.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q isn't a number", raw)
		}
		f.SetFloat(n)
	default:
		return fmt.Errorf("settings can't read a %s", f.Type())
	}
	return nil
}
