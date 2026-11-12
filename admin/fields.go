package admin

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Unfield/Tern/internal/formrules"
)

// field is a struct field as the admin screens see it.
type field struct {
	rules    formrules.Field // named after the column
	index    []int
	typ      reflect.Type
	column   string
	input    string
	options  []string
	help     string
	ref      string
	key      bool
	list     bool
	search   bool
	filter   bool
	readonly bool
	nullable bool
	// epoch is the unit of an integer that holds a time, or 0.
	epoch time.Duration
}

const (
	dateLayout     = "2006-01-02"
	datetimeLayout = "2006-01-02T15:04"
)

// fieldsOf reads T's exported fields. The column is the db tag, else the
// field's name in snake_case; the admin tag says how the field is shown.
func fieldsOf(t reflect.Type) ([]field, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("admin: %s is not a struct", t)
	}
	var out []field
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() || sf.Tag.Get("admin") == "-" || sf.Tag.Get("db") == "-" {
			continue
		}
		f := field{index: sf.Index, typ: sf.Type, column: sf.Tag.Get("db"), help: sf.Tag.Get("description")}
		if f.column == "" {
			f.column = snake(sf.Name)
		}
		dateOnly := false
		for opt := range strings.SplitSeq(sf.Tag.Get("admin"), ",") {
			name, arg, _ := strings.Cut(strings.TrimSpace(opt), "=")
			switch name {
			case "":
			case "key":
				f.key = true
			case "list":
				f.list = true
			case "search":
				f.search = true
			case "filter":
				f.filter = true
			case "readonly":
				f.readonly = true
			case "textarea":
				f.input = "textarea"
			case "date":
				dateOnly = true
			case "ref":
				f.ref = arg
			case "unixms":
				f.epoch = time.Millisecond
			case "unix":
				f.epoch = time.Second
			default:
				return nil, fmt.Errorf("admin: field %s: unknown option %q in its admin tag", sf.Name, name)
			}
		}
		base := sf.Type
		if base.Kind() == reflect.Pointer {
			base, f.nullable = base.Elem(), true
		}
		kind, err := formrules.KindOf(base, sf.Tag)
		if err != nil || kind == formrules.Strings {
			return nil, fmt.Errorf("admin: field %s: type %s isn't supported; use a string, number, bool or time.Time, or a pointer to one", sf.Name, sf.Type)
		}
		if f.epoch != 0 {
			if kind != formrules.Int {
				return nil, fmt.Errorf("admin: field %s: unix and unixms are for integers", sf.Name)
			}
			kind = formrules.Date
		}
		// Times in a database are mostly moments, not days.
		if kind == formrules.Date && !dateOnly {
			kind = formrules.DateTime
		}
		secret := strings.Contains(strings.ToLower(f.column), "password")
		f.rules, err = formrules.New(f.column, kind, secret, sf.Tag, sf.Index)
		if err != nil {
			return nil, fmt.Errorf("admin: field %s: %w", sf.Name, err)
		}
		if sf.Tag.Get("label") == "" {
			f.rules.Label = label(sf.Name)
		}
		if f.input == "" {
			f.input = inputOf(f.rules)
		}
		for _, r := range f.rules.Rules {
			if r.Name == "oneof" {
				f.options = strings.Split(r.Arg, "|")
				f.input = "select"
			}
		}
		if f.search && kind != formrules.String {
			return nil, fmt.Errorf("admin: field %s: only text can be searched", sf.Name)
		}
		out = append(out, f)
	}
	keys := 0
	for _, f := range out {
		if f.key {
			keys++
		}
	}
	if keys == 0 {
		for i := range out {
			if out[i].column == "id" {
				out[i].key = true
				keys = 1
			}
		}
	}
	if keys != 1 {
		return nil, fmt.Errorf("admin: %s needs one key field: tag it admin:\"key\", or name it ID", t)
	}
	listed := slices.ContainsFunc(out, func(f field) bool { return f.list })
	for i := range out {
		if out[i].key {
			// A key is set once: by the database for numbers, by the user
			// when the record is created otherwise.
			out[i].list = true
			if out[i].rules.Kind == formrules.String {
				out[i].rules.Required = true
			}
		}
		// Without list tags, the list shows the first few fields.
		if !listed && i < 5 && out[i].input != "textarea" {
			out[i].list = true
		}
		if out[i].rules.Secret {
			out[i].list = false
		}
	}
	return out, nil
}

func inputOf(f formrules.Field) string {
	switch f.Kind {
	case formrules.Bool:
		return "checkbox"
	case formrules.Int, formrules.Float:
		return "number"
	case formrules.Date:
		return "date"
	case formrules.DateTime:
		return "datetime-local"
	}
	if f.Secret {
		return "password"
	}
	for _, r := range f.Rules {
		switch r.Name {
		case "email":
			return "email"
		case "url":
			return "url"
		}
	}
	return "text"
}

// snake turns a Go name into a column name: AddedAt is added_at, and URL is url.
func snake(name string) string {
	var b strings.Builder
	rs := []rune(name)
	for i, r := range rs {
		if unicode.IsUpper(r) {
			if i > 0 && (unicode.IsLower(rs[i-1]) || i+1 < len(rs) && unicode.IsLower(rs[i+1])) {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// text formats a field's value as a form posts it.
func (f *field) text(v reflect.Value) string {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if f.epoch != 0 {
		if v.Int() == 0 {
			return ""
		}
		v = reflect.ValueOf(time.UnixMilli(v.Int() * int64(f.epoch/time.Millisecond)))
	}
	switch f.rules.Kind {
	case formrules.Bool:
		if v.Bool() {
			return "true"
		}
		return ""
	case formrules.Date, formrules.DateTime:
		t := v.Interface().(time.Time)
		if t.IsZero() {
			return ""
		}
		if f.rules.Kind == formrules.Date {
			return t.Format(dateLayout)
		}
		return t.UTC().Format(datetimeLayout)
	case formrules.Int:
		if v.CanInt() {
			return strconv.FormatInt(v.Int(), 10)
		}
		return strconv.FormatUint(v.Uint(), 10)
	case formrules.Float:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	}
	return v.String()
}

// set parses s, as a form posted it, into v.
func (f *field) set(v reflect.Value, s string) error {
	s = strings.TrimSpace(s)
	if v.Kind() == reflect.Pointer {
		if s == "" && f.rules.Kind != formrules.Bool {
			v.SetZero()
			return nil
		}
		v.Set(reflect.New(v.Type().Elem()))
		v = v.Elem()
	}
	switch f.rules.Kind {
	case formrules.Bool:
		v.SetBool(formrules.Truthy(s))
	case formrules.Date, formrules.DateTime:
		if s == "" {
			v.SetZero()
			return nil
		}
		t, ok := formrules.ParseTime(f.rules.Kind, s)
		if !ok {
			return fmt.Errorf("not a valid date")
		}
		if f.epoch != 0 {
			v.SetInt(t.UnixMilli() / int64(f.epoch/time.Millisecond))
		} else {
			v.Set(reflect.ValueOf(t))
		}
	case formrules.Int:
		if s == "" {
			v.SetZero()
			return nil
		}
		if v.CanInt() {
			n, err := strconv.ParseInt(s, 10, v.Type().Bits())
			if err != nil {
				return err
			}
			v.SetInt(n)
		} else {
			n, err := strconv.ParseUint(s, 10, v.Type().Bits())
			if err != nil {
				return err
			}
			v.SetUint(n)
		}
	case formrules.Float:
		if s == "" {
			v.SetZero()
			return nil
		}
		n, err := strconv.ParseFloat(s, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetFloat(n)
	default:
		v.SetString(s)
	}
	return nil
}

// label turns a Go name into a label, keeping acronyms: UserID is "User
// ID", and AddedAt is "Added at".
func label(name string) string {
	var words []string
	rs := []rune(name)
	start := 0
	for i := 1; i <= len(rs); i++ {
		if i == len(rs) || unicode.IsUpper(rs[i]) && (unicode.IsLower(rs[i-1]) || i+1 < len(rs) && unicode.IsLower(rs[i+1])) {
			w := string(rs[start:i])
			if !acronym(w) {
				w = strings.ToLower(w)
			}
			words = append(words, w)
			start = i
		}
	}
	s := strings.Join(words, " ")
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func acronym(w string) bool {
	return len([]rune(w)) > 1 && strings.ToUpper(w) == w
}
