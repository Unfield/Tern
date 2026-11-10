// Package i18n formats translated messages. It implements the same message
// syntax and plural rules as the client runtime (packages/tern/src/runtime/
// i18n), and both are tested against testdata/cases.json, so the server and
// the browser always produce the same text.
//
// Messages support a subset of ICU MessageFormat:
//
//	Hello, {name}!
//	{count, plural, =0 {No items} one {# item} other {# items}}
//	{role, select, admin {Administrator} other {Member}}
//
// Quote braces with apostrophes ('{'), and write an apostrophe as ”.
package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Catalog holds every locale's messages, flattened to dotted keys.
type Catalog struct {
	Default  string
	Locales  []string
	messages map[string]map[string]string
}

// Load reads <locale>.json files from fsys. defaultLocale must be one of
// them; if it is "", "en" is used when present, else the first locale.
func Load(fsys fs.FS, defaultLocale string) (*Catalog, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	c := &Catalog{messages: map[string]map[string]string{}}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || path.Ext(name) != ".json" {
			continue
		}
		locale := strings.TrimSuffix(name, ".json")
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		msgs, err := Flatten(raw)
		if err != nil {
			return nil, fmt.Errorf("i18n: %s: %w", name, err)
		}
		c.messages[locale] = msgs
		c.Locales = append(c.Locales, locale)
	}
	sort.Strings(c.Locales)
	if len(c.Locales) == 0 {
		return nil, fmt.Errorf("i18n: no locale files")
	}
	switch {
	case defaultLocale != "":
		if c.messages[defaultLocale] == nil {
			return nil, fmt.Errorf("i18n: the default locale %q has no messages file", defaultLocale)
		}
		c.Default = defaultLocale
	case c.messages["en"] != nil:
		c.Default = "en"
	default:
		c.Default = c.Locales[0]
	}
	return c, nil
}

// Flatten turns nested JSON objects into dotted keys: {"a": {"b": "x"}}
// becomes {"a.b": "x"}. Values must be strings.
func Flatten(raw []byte) (map[string]string, error) {
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}
	out := map[string]string{}
	var walk func(prefix string, v any) error
	walk = func(prefix string, v any) error {
		switch t := v.(type) {
		case string:
			if _, err := parse(t); err != nil {
				return fmt.Errorf("message %q: %w", prefix, err)
			}
			out[prefix] = t
		case map[string]any:
			for k, child := range t {
				key := k
				if prefix != "" {
					key = prefix + "." + k
				}
				if err := walk(key, child); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("message %q must be a string or an object", prefix)
		}
		return nil
	}
	return out, walk("", tree)
}

// Keys returns the default locale's message keys, sorted.
func (c *Catalog) Keys() []string {
	keys := make([]string, 0, len(c.messages[c.Default]))
	for k := range c.messages[c.Default] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Missing returns, for each other locale, the default locale's keys it lacks.
func (c *Catalog) Missing() map[string][]string {
	out := map[string][]string{}
	for _, l := range c.Locales {
		if l == c.Default {
			continue
		}
		for _, k := range c.Keys() {
			if _, ok := c.messages[l][k]; !ok {
				out[l] = append(out[l], k)
			}
		}
	}
	return out
}

// Has reports whether locale has a messages file.
func (c *Catalog) Has(locale string) bool { return c.messages[locale] != nil }

// T formats key in locale, falling back to the default locale and then to
// the key itself.
func (c *Catalog) T(locale, key string, params map[string]any) string {
	msg, ok := c.messages[locale][key]
	if !ok {
		if msg, ok = c.messages[c.Default][key]; !ok {
			return key
		}
	}
	return Format(locale, msg, params)
}

// Match picks the best supported locale for an Accept-Language header,
// or "" if none fits.
func (c *Catalog) Match(acceptLanguage string) string {
	type pref struct {
		tag string
		q   float64
	}
	var prefs []pref
	for _, part := range strings.Split(acceptLanguage, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			q, _ = strconv.ParseFloat(v, 64)
		}
		if tag != "" && q > 0 {
			prefs = append(prefs, pref{strings.TrimSpace(tag), q})
		}
	}
	sort.SliceStable(prefs, func(i, j int) bool { return prefs[i].q > prefs[j].q })
	for _, p := range prefs {
		for _, l := range c.Locales { // exact match, ignoring case
			if strings.EqualFold(l, p.tag) {
				return l
			}
		}
		base, _, _ := strings.Cut(p.tag, "-")
		for _, l := range c.Locales { // "de-AT" matches "de"
			if strings.EqualFold(l, base) {
				return l
			}
		}
	}
	return ""
}

// Format formats one message.
func Format(locale, msg string, params map[string]any) string {
	nodes, err := parse(msg)
	if err != nil {
		return msg
	}
	var b strings.Builder
	render(&b, nodes, locale, params, nil)
	return b.String()
}

type node struct {
	text    string
	arg     string // {arg}
	kind    string // "", "plural", "select"
	options map[string][]node
	order   []string
}

func render(b *strings.Builder, nodes []node, locale string, params map[string]any, hash *float64) {
	for _, n := range nodes {
		switch {
		case n.arg == "" && n.text == "#" && hash != nil:
			b.WriteString(formatNumber(*hash))
		case n.arg == "":
			b.WriteString(n.text)
		case n.kind == "":
			v, ok := params[n.arg]
			if !ok {
				b.WriteString("{" + n.arg + "}")
				continue
			}
			fmt.Fprint(b, v)
		case n.kind == "plural":
			num, _ := toNumber(params[n.arg])
			body, ok := n.options["="+formatNumber(num)]
			if !ok {
				body, ok = n.options[pluralCategory(locale, num)]
			}
			if !ok {
				body = n.options["other"]
			}
			render(b, body, locale, params, &num)
		case n.kind == "select":
			body, ok := n.options[fmt.Sprint(params[n.arg])]
			if !ok {
				body = n.options["other"]
			}
			render(b, body, locale, params, hash)
		}
	}
}

func toNumber(v any) (float64, bool) {
	switch t := v.(type) {
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	}
	return 0, false
}

func formatNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// parse turns a message into nodes. The grammar matches the client's.
func parse(msg string) ([]node, error) {
	p := &parser{s: []rune(msg)}
	nodes, err := p.nodes(false, false)
	if err != nil {
		return nil, err
	}
	if p.i < len(p.s) {
		return nil, fmt.Errorf("unexpected '}' at %d", p.i)
	}
	return nodes, nil
}

type parser struct {
	s []rune
	i int
}

func (p *parser) nodes(inBranch, plural bool) ([]node, error) {
	var out []node
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			out = append(out, node{text: text.String()})
			text.Reset()
		}
	}
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '\'':
			// '' is an apostrophe; '{...}' quotes syntax characters.
			if p.i+1 < len(p.s) && p.s[p.i+1] == '\'' {
				text.WriteRune('\'')
				p.i += 2
				continue
			}
			if p.i+1 < len(p.s) && strings.ContainsRune("{}#", p.s[p.i+1]) {
				end := p.i + 1
				for end < len(p.s) && p.s[end] != '\'' {
					end++
				}
				text.WriteString(string(p.s[p.i+1 : end]))
				p.i = end + 1
				continue
			}
			text.WriteRune(c)
			p.i++
		case c == '{':
			flush()
			n, err := p.argument()
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		case c == '}':
			if !inBranch {
				return nil, fmt.Errorf("unexpected '}' at %d", p.i)
			}
			flush()
			return out, nil
		case c == '#' && plural:
			flush()
			out = append(out, node{text: "#"})
			p.i++
		default:
			text.WriteRune(c)
			p.i++
		}
	}
	if inBranch {
		return nil, fmt.Errorf("unclosed '{'")
	}
	flush()
	return out, nil
}

func (p *parser) argument() (node, error) {
	p.i++ // {
	name := strings.TrimSpace(p.until(",}"))
	if name == "" {
		return node{}, fmt.Errorf("empty argument at %d", p.i)
	}
	if p.i >= len(p.s) {
		return node{}, fmt.Errorf("unclosed '{'")
	}
	if p.s[p.i] == '}' {
		p.i++
		return node{arg: name}, nil
	}
	p.i++ // ,
	kind := strings.TrimSpace(p.until(",}"))
	if kind != "plural" && kind != "select" {
		return node{}, fmt.Errorf("unknown argument type %q", kind)
	}
	if p.i >= len(p.s) || p.s[p.i] != ',' {
		return node{}, fmt.Errorf("%s argument %q needs options", kind, name)
	}
	p.i++
	n := node{arg: name, kind: kind, options: map[string][]node{}}
	for {
		p.skipSpace()
		if p.i >= len(p.s) {
			return node{}, fmt.Errorf("unclosed '{'")
		}
		if p.s[p.i] == '}' {
			p.i++
			break
		}
		key := strings.TrimSpace(p.until("{"))
		if key == "" || p.i >= len(p.s) {
			return node{}, fmt.Errorf("expected an option in %q", name)
		}
		p.i++ // {
		body, err := p.nodes(true, kind == "plural")
		if err != nil {
			return node{}, err
		}
		p.i++ // }
		n.options[key] = body
		n.order = append(n.order, key)
	}
	if _, ok := n.options["other"]; !ok {
		return node{}, fmt.Errorf("%s argument %q needs an 'other' option", kind, name)
	}
	return n, nil
}

func (p *parser) until(stops string) string {
	start := p.i
	for p.i < len(p.s) && !strings.ContainsRune(stops, p.s[p.i]) {
		p.i++
	}
	return string(p.s[start:p.i])
}

func (p *parser) skipSpace() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\n' || p.s[p.i] == '\t') {
		p.i++
	}
}

// pluralCategory returns the CLDR cardinal plural category of n in locale.
// It covers integers and common decimals for the most widely used
// languages; anything else uses one/other.
func pluralCategory(locale string, n float64) string {
	lang, _, _ := strings.Cut(strings.ToLower(locale), "-")
	lang, _, _ = strings.Cut(lang, "_")
	i := math.Abs(math.Trunc(n))
	isInt := n == math.Trunc(n)
	mod10, mod100 := math.Mod(i, 10), math.Mod(i, 100)
	switch lang {
	case "ja", "zh", "ko", "vi", "th", "id", "ms", "lo", "my", "km":
		return "other"
	case "fr", "pt", "hy", "ff", "kab":
		if i == 0 || i == 1 {
			return "one"
		}
	case "ru", "uk", "be", "sr", "hr", "bs":
		if !isInt {
			return "other"
		}
		switch {
		case mod10 == 1 && mod100 != 11:
			return "one"
		case mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14):
			return "few"
		case lang == "ru" || lang == "uk" || lang == "be":
			return "many"
		}
	case "pl":
		if !isInt {
			return "other"
		}
		switch {
		case i == 1:
			return "one"
		case mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14):
			return "few"
		default:
			return "many"
		}
	case "cs", "sk":
		switch {
		case !isInt:
			return "many"
		case i == 1:
			return "one"
		case i >= 2 && i <= 4:
			return "few"
		}
	case "ro":
		switch {
		case i == 1 && isInt:
			return "one"
		case !isInt || i == 0 || (mod100 >= 2 && mod100 <= 19):
			return "few"
		}
	case "lt":
		switch {
		case mod10 == 1 && (mod100 < 11 || mod100 > 19):
			return "one"
		case mod10 >= 2 && (mod100 < 11 || mod100 > 19):
			return "few"
		}
	case "lv":
		switch {
		case mod10 == 0 || (mod100 >= 11 && mod100 <= 19):
			return "zero"
		case mod10 == 1 && mod100 != 11:
			return "one"
		}
	case "ar":
		switch {
		case n == 0:
			return "zero"
		case n == 1:
			return "one"
		case n == 2:
			return "two"
		case mod100 >= 3 && mod100 <= 10:
			return "few"
		case mod100 >= 11 && mod100 <= 99:
			return "many"
		}
	case "he", "iw":
		switch {
		case i == 1 && isInt:
			return "one"
		case i == 2 && isInt:
			return "two"
		}
	case "ga":
		switch {
		case n == 1:
			return "one"
		case n == 2:
			return "two"
		case n >= 3 && n <= 6:
			return "few"
		case n >= 7 && n <= 10:
			return "many"
		}
	case "cy":
		switch n {
		case 0:
			return "zero"
		case 1:
			return "one"
		case 2:
			return "two"
		case 3:
			return "few"
		case 6:
			return "many"
		}
	default: // en, de, nl, sv, da, no, nb, it, es, fi, et, el, hu, tr, bg, ca, ...
		if n == 1 && isInt {
			return "one"
		}
	}
	return "other"
}
