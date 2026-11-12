package mcp

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/internal/formrules"
)

// Schemas are JSON Schema (2020-12), built from Go types by reflection:
// json names, `description` tags, and the validate rules forms use.

var (
	timeType      = reflect.TypeFor[time.Time]()
	rawType       = reflect.TypeFor[json.RawMessage]()
	marshalerType = reflect.TypeFor[json.Marshaler]()
)

// schemaOf returns the schema of t. forInput reads validate tags for
// required fields and limits; for output, a field is required unless it
// has omitempty.
func schemaOf(t reflect.Type, forInput bool) map[string]any {
	return (&schemaBuilder{input: forInput, seen: map[reflect.Type]bool{}}).build(t, "")
}

type schemaBuilder struct {
	input bool
	seen  map[reflect.Type]bool // structs being built, to stop at recursion
}

func (b *schemaBuilder) build(t reflect.Type, validate string) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	s := map[string]any{}
	switch {
	case t == timeType:
		s["type"] = "string"
		s["format"] = "date-time"
		return s
	case t == rawType:
		return s
	case t.Implements(marshalerType) || reflect.PointerTo(t).Implements(marshalerType):
		// A type that writes its own JSON could be anything.
		return s
	}
	switch t.Kind() {
	case reflect.String:
		s["type"] = "string"
	case reflect.Bool:
		s["type"] = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		s["type"] = "integer"
	case reflect.Float32, reflect.Float64:
		s["type"] = "number"
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			s["type"] = "string" // []byte is base64
			s["contentEncoding"] = "base64"
			break
		}
		s["type"] = "array"
		s["items"] = b.build(t.Elem(), "")
	case reflect.Map:
		s["type"] = "object"
		s["additionalProperties"] = b.build(t.Elem(), "")
	case reflect.Struct:
		if b.seen[t] {
			s["type"] = "object" // recursive: described once, above
			return s
		}
		b.seen[t] = true
		defer delete(b.seen, t)
		b.object(t, s)
	case reflect.Interface:
		// any JSON value
	}
	b.limits(t, validate, s)
	return s
}

func (b *schemaBuilder) object(t reflect.Type, s map[string]any) {
	props := map[string]any{}
	var required []string
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for i := range t.NumField() {
			f := t.Field(i)
			name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" && opts == "" {
				continue
			}
			if f.Anonymous && name == "" {
				ft := f.Type
				if ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct {
					walk(ft) // embedded fields are the outer struct's
					continue
				}
			}
			if !f.IsExported() {
				continue
			}
			if name == "" {
				name = f.Name
			}
			validate := f.Tag.Get("validate")
			p := b.build(f.Type, validate)
			if d := f.Tag.Get("description"); d != "" {
				p["description"] = d
			}
			props[name] = p
			if b.input {
				if req, _, _ := formrules.ParseTag(validate); req {
					required = append(required, name)
				}
			} else if !strings.Contains(opts, "omitempty") && !strings.Contains(opts, "omitzero") {
				required = append(required, name)
			}
		}
	}
	walk(t)
	s["type"] = "object"
	s["properties"] = props
	if len(required) > 0 {
		s["required"] = required
	}
	if b.input {
		s["additionalProperties"] = false
	}
}

// limits adds a field's validate rules: lengths, ranges, formats, choices.
func (b *schemaBuilder) limits(t reflect.Type, validate string, s map[string]any) {
	if validate == "" {
		return
	}
	_, rules, err := formrules.ParseTag(validate)
	if err != nil {
		return
	}
	typ, _ := s["type"].(string)
	num := func(arg string) any {
		if n, err := strconv.ParseInt(arg, 10, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(arg, 64); err == nil {
			return f
		}
		return nil
	}
	for _, r := range rules {
		switch r.Name {
		case "min", "max", "len":
			n := num(r.Arg)
			if n == nil {
				continue // a date limit, say
			}
			var key string
			switch typ {
			case "string":
				key = map[string]string{"min": "minLength", "max": "maxLength", "len": "minLength"}[r.Name]
				if r.Name == "len" {
					s["maxLength"] = n
				}
			case "integer", "number":
				key = map[string]string{"min": "minimum", "max": "maximum"}[r.Name]
			case "array":
				key = map[string]string{"min": "minItems", "max": "maxItems", "len": "minItems"}[r.Name]
				if r.Name == "len" {
					s["maxItems"] = n
				}
			}
			if key != "" {
				s[key] = n
			}
		case "email":
			s["format"] = "email"
		case "url":
			s["format"] = "uri"
		case "pattern":
			s["pattern"] = "^(?:" + r.Arg + ")$"
		case "oneof":
			choices := strings.Split(r.Arg, "|")
			if typ == "array" {
				if items, ok := s["items"].(map[string]any); ok {
					items["enum"] = choices
				}
			} else {
				s["enum"] = choices
			}
		}
	}
	_ = t
}
