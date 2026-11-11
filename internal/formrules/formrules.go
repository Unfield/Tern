// Package formrules parses validation tags and checks submitted form values
// against them. kit uses it at run time; codegen uses it to write the same
// rules for the browser, where packages/tern/src/runtime/validate.ts applies
// them with the same messages. testdata/cases.json keeps the two in step.
package formrules

import (
	"fmt"
	"go/types"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Unfield/Tern/internal/i18n"
)

// Kind is how a field's text is read.
type Kind string

// The kinds of field, from the Go type: text, whole and decimal numbers,
// checkboxes, dates and lists.
const (
	String   Kind = "string"
	Int      Kind = "int"
	Float    Kind = "float"
	Bool     Kind = "bool"
	Date     Kind = "date"     // YYYY-MM-DD
	DateTime Kind = "datetime" // YYYY-MM-DDTHH:MM, as datetime-local inputs post
	Strings  Kind = "strings"  // several values, such as checkboxes with one name
)

// Rule is one check from a validate tag, such as min=3.
type Rule struct {
	Name string `json:"name"`
	Arg  string `json:"arg,omitempty"`
}

// Field is a form field's rules.
type Field struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Kind     Kind   `json:"kind"`
	Required bool   `json:"required,omitempty"`
	Rules    []Rule `json:"rules,omitempty"`
	// Message replaces every failure message for the field.
	Message string `json:"message,omitempty"`
	// Secret fields, such as passwords, are never sent back to refill a form.
	Secret bool `json:"secret,omitempty"`

	index []int
	re    *regexp.Regexp
}

// Index is the field's position in its struct, for reflect.Value.FieldByIndex.
func (f *Field) Index() []int { return f.index }

var known = map[string]bool{"required": true, "min": true, "max": true, "len": true, "email": true, "url": true, "pattern": true, "oneof": true, "same": true}

// ParseTag reads a validate tag: comma-separated rules, where pattern= takes
// the rest of the tag so its regular expression may contain commas.
func ParseTag(tag string) (required bool, rules []Rule, err error) {
	for tag != "" {
		var part string
		if strings.HasPrefix(tag, "pattern=") {
			part, tag = tag, ""
		} else {
			part, tag, _ = strings.Cut(tag, ",")
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, arg, _ := strings.Cut(part, "=")
		if !known[name] {
			return false, nil, fmt.Errorf("unknown rule %q", name)
		}
		switch name {
		case "required":
			required = true
			continue
		case "min", "max", "len":
			if arg == "" {
				return false, nil, fmt.Errorf("%s needs a value, as in %s=3", name, name)
			}
		case "pattern":
			if _, err := regexp.Compile("^(?:" + arg + ")$"); err != nil {
				return false, nil, fmt.Errorf("pattern: %w", err)
			}
		case "oneof", "same":
			if arg == "" {
				return false, nil, fmt.Errorf("%s needs a value", name)
			}
		}
		rules = append(rules, Rule{Name: name, Arg: arg})
	}
	return required, rules, nil
}

// Humanize turns a field name into a label: "firstName" and "first_name"
// become "First name".
func Humanize(name string) string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	rs := []rune(name)
	for i, r := range rs {
		switch {
		case r == '_' || r == '-' || r == '.' || r == ' ':
			flush()
		case unicode.IsUpper(r) && i > 0 && (unicode.IsLower(rs[i-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]))):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	if len(words) == 0 {
		return name
	}
	s := strings.Join(words, " ")
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

// formName returns the posted name of a struct field and its form tag options.
func formName(goName, formTag, jsonTag string) (name string, secret, skip bool) {
	if formTag == "-" {
		return "", false, true
	}
	if formTag != "" {
		n, opts, _ := strings.Cut(formTag, ",")
		secret = strings.Contains(","+opts+",", ",secret,")
		if n != "" {
			name = n
		}
	}
	if name == "" && jsonTag != "" && jsonTag != "-" {
		name, _, _ = strings.Cut(jsonTag, ",")
	}
	if name == "" {
		name = goName
	}
	if strings.Contains(strings.ToLower(name), "password") {
		secret = true
	}
	return name, secret, false
}

var (
	timeType   = reflect.TypeFor[time.Time]()
	stringType = reflect.TypeFor[string]()
)

func kindOf(t reflect.Type, tag reflect.StructTag) (Kind, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == timeType {
		if strings.Contains(tag.Get("form"), ",datetime") {
			return DateTime, nil
		}
		return Date, nil
	}
	switch t.Kind() {
	case reflect.String:
		return String, nil
	case reflect.Bool:
		return Bool, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Int, nil
	case reflect.Float32, reflect.Float64:
		return Float, nil
	case reflect.Slice:
		if t.Elem() == stringType || t.Elem().Kind() == reflect.String {
			return Strings, nil
		}
	}
	return "", fmt.Errorf("type %s can't come from a form; use a string, number, bool, time.Time or []string", t)
}

// FromReflect reads the fields of a struct type for decoding and checking.
func FromReflect(t reflect.Type) ([]Field, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%s is not a struct", t)
	}
	var out []Field
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		name, secret, skip := formName(sf.Name, sf.Tag.Get("form"), sf.Tag.Get("json"))
		if skip {
			continue
		}
		kind, err := kindOf(sf.Type, sf.Tag)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", sf.Name, err)
		}
		f, err := build(name, kind, secret, sf.Tag)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", sf.Name, err)
		}
		f.index = sf.Index
		out = append(out, f)
	}
	return finish(out), nil
}

// FromTypes reads the fields of a struct type as go/types sees it, for codegen.
func FromTypes(t types.Type) ([]Field, error) {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	st, ok := types.Unalias(t).Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("%s is not a struct", t)
	}
	var out []Field
	for i := range st.NumFields() {
		v := st.Field(i)
		if !v.Exported() {
			continue
		}
		tag := reflect.StructTag(st.Tag(i))
		name, secret, skip := formName(v.Name(), tag.Get("form"), tag.Get("json"))
		if skip {
			continue
		}
		kind, err := typesKind(v.Type(), tag)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", v.Name(), err)
		}
		f, err := build(name, kind, secret, tag)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", v.Name(), err)
		}
		out = append(out, f)
	}
	return finish(out), nil
}

// finish marks a field secret when it must equal a secret field, as a
// password confirmation must.
func finish(fields []Field) []Field {
	secret := map[string]bool{}
	for _, f := range fields {
		secret[f.Name] = f.Secret
	}
	for i, f := range fields {
		for _, r := range f.Rules {
			if r.Name == "same" && secret[r.Arg] {
				fields[i].Secret = true
			}
		}
	}
	return fields
}

func typesKind(t types.Type, tag reflect.StructTag) (Kind, error) {
	for {
		p, ok := types.Unalias(t).(*types.Pointer)
		if !ok {
			break
		}
		t = p.Elem()
	}
	if n, ok := types.Unalias(t).(*types.Named); ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "time" && n.Obj().Name() == "Time" {
		if strings.Contains(tag.Get("form"), ",datetime") {
			return DateTime, nil
		}
		return Date, nil
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsString != 0:
			return String, nil
		case u.Kind() == types.Bool:
			return Bool, nil
		case u.Info()&types.IsInteger != 0:
			return Int, nil
		case u.Info()&types.IsFloat != 0:
			return Float, nil
		}
	case *types.Slice:
		if b, ok := u.Elem().Underlying().(*types.Basic); ok && b.Info()&types.IsString != 0 {
			return Strings, nil
		}
	}
	return "", fmt.Errorf("type %s can't come from a form; use a string, number, bool, time.Time or []string", t)
}

func build(name string, kind Kind, secret bool, tag reflect.StructTag) (Field, error) {
	required, rules, err := ParseTag(tag.Get("validate"))
	if err != nil {
		return Field{}, err
	}
	f := Field{Name: name, Label: tag.Get("label"), Kind: kind, Required: required, Rules: rules, Message: tag.Get("message"), Secret: secret}
	if f.Label == "" {
		f.Label = Humanize(name)
	}
	for _, r := range rules {
		switch r.Name {
		case "min", "max", "len":
			if kind == Date || kind == DateTime {
				if !validDate(kind, r.Arg) {
					return Field{}, fmt.Errorf("%s=%s is not a valid %s", r.Name, r.Arg, kind)
				}
			} else if _, err := strconv.ParseFloat(r.Arg, 64); err != nil {
				return Field{}, fmt.Errorf("%s=%s is not a number", r.Name, r.Arg)
			}
		case "pattern":
			f.re = regexp.MustCompile("^(?:" + r.Arg + ")$")
		}
	}
	return f, nil
}

func validDate(kind Kind, s string) bool {
	_, ok := ParseTime(kind, s)
	return ok
}

// ParseTime reads a date, or a date and time, as forms post them.
func ParseTime(kind Kind, s string) (time.Time, bool) {
	layouts := []string{time.DateOnly}
	if kind == DateTime {
		layouts = []string{"2006-01-02T15:04", "2006-01-02T15:04:05", time.RFC3339}
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Truthy reports whether a checkbox's value means checked.
func Truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off", "false", "0", "no":
		return false
	}
	return true
}

// The same expressions are in validate.ts.
var (
	emailRe = regexp.MustCompile(`^[^\s@<>()",;:]+@[^\s@<>()",;:]+\.[^\s@<>()",;:]+$`)
	intRe   = regexp.MustCompile(`^[+-]?\d+$`)
	floatRe = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)$`)
)

// Check returns why the posted values of f are invalid, or "". values are
// the field's posted values; other returns another field's first value,
// for same=. t translates Tern's texts (i18n.Catalog.TB); nil uses English.
func Check(f Field, values []string, other func(name string) string, labelOf func(name string) string, t func(key string, params map[string]any) string) string {
	if t == nil {
		var none *i18n.Catalog
		t = func(key string, params map[string]any) string { return none.TB("en", key, params) }
	}
	msg := func(key string, params map[string]any) string {
		if f.Message != "" {
			return f.Message
		}
		if params == nil {
			params = map[string]any{}
		}
		params["field"] = f.Label
		params["fieldLower"] = strings.ToLower(f.Label)
		return t("tern.validate."+key, params)
	}
	var list []string
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			list = append(list, v)
		}
	}
	v := ""
	if len(values) > 0 {
		v = values[0]
	}
	var empty bool
	switch f.Kind {
	case Strings:
		empty = len(list) == 0
	case Bool:
		empty = !Truthy(v)
	default:
		empty = strings.TrimSpace(v) == ""
	}
	if empty {
		if !f.Required {
			return ""
		}
		if f.Kind == Bool {
			return msg("checked", nil)
		}
		return msg("required", nil)
	}
	var num float64
	var when time.Time
	switch f.Kind {
	case Int:
		tv := strings.TrimSpace(v)
		if !intRe.MatchString(tv) {
			return msg("int", nil)
		}
		num, _ = strconv.ParseFloat(tv, 64)
	case Float:
		tv := strings.TrimSpace(v)
		if !floatRe.MatchString(tv) {
			return msg("number", nil)
		}
		num, _ = strconv.ParseFloat(tv, 64)
	case Date, DateTime:
		tv, ok := ParseTime(f.Kind, strings.TrimSpace(v))
		if !ok {
			if f.Kind == Date {
				return msg("date", nil)
			}
			return msg("datetime", nil)
		}
		when = tv
	}
	for _, r := range f.Rules {
		n, _ := strconv.ParseFloat(r.Arg, 64)
		bound := map[string]any{"count": r.Arg, "n": n}
		switch r.Name {
		case "min", "max", "len":
			low := r.Name == "min"
			switch f.Kind {
			case String:
				c := float64(utf8.RuneCountInString(v))
				if (r.Name == "len" && c != n) || (low && c < n) || (r.Name == "max" && c > n) {
					return msg(map[string]string{"min": "minLength", "max": "maxLength", "len": "length"}[r.Name], bound)
				}
			case Int, Float:
				if (r.Name == "len" && num != n) || (low && num < n) || (r.Name == "max" && num > n) {
					return msg(map[string]string{"min": "min", "max": "max", "len": "exactly"}[r.Name], bound)
				}
			case Date, DateTime:
				b, _ := ParseTime(f.Kind, r.Arg)
				if (low && when.Before(b)) || (r.Name == "max" && when.After(b)) {
					key := "before"
					if low {
						key = "after"
					}
					return msg(key, map[string]any{"date": r.Arg})
				}
			case Strings:
				c := float64(len(list))
				if (r.Name == "len" && c != n) || (low && c < n) || (r.Name == "max" && c > n) {
					return msg(map[string]string{"min": "minItems", "max": "maxItems", "len": "items"}[r.Name], bound)
				}
			}
		case "email":
			if !emailRe.MatchString(v) {
				return msg("email", nil)
			}
		case "url":
			u, err := url.Parse(v)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsAny(v, " \t\n") {
				return msg("url", nil)
			}
		case "pattern":
			for _, s := range valuesFor(f, v, list) {
				if !f.re.MatchString(s) {
					return msg("pattern", nil)
				}
			}
		case "oneof":
			opts := strings.Split(r.Arg, "|")
			for _, s := range valuesFor(f, v, list) {
				if !contains(opts, s) {
					return msg("oneof", map[string]any{"options": joinOr(opts, t("tern.validate.or", nil))})
				}
			}
		case "same":
			if other(r.Arg) != v {
				return msg("same", map[string]any{"otherLower": strings.ToLower(labelOf(r.Arg))})
			}
		}
	}
	return ""
}

func valuesFor(f Field, v string, list []string) []string {
	if f.Kind == Strings {
		return list
	}
	return []string{v}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func joinOr(list []string, or string) string {
	if len(list) == 1 {
		return list[0]
	}
	return strings.Join(list[:len(list)-1], ", ") + " " + or + " " + list[len(list)-1]
}
