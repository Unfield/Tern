package routes

import (
	"fmt"
	"regexp"
	"strings"
)

// Kind is the kind of a route segment.
type Kind int

const (
	// Static is literal text, such as "blog".
	Static Kind = iota
	// Param is a required parameter: "slug.param".
	Param
	// OptionalParam is a parameter that may be absent: "lang.optional".
	OptionalParam
	// Rest matches zero or more path segments: "path.rest".
	Rest
)

func (k Kind) String() string {
	switch k {
	case Static:
		return "static"
	case Param:
		return "param"
	case OptionalParam:
		return "optional"
	case Rest:
		return "rest"
	default:
		return fmt.Sprintf("Kind(%d)", int(k))
	}
}

// Segment is one URL path segment, parsed from a route directory name.
type Segment struct {
	// Raw is the directory name, e.g. "slug.param".
	Raw  string
	Kind Kind
	// Value is the literal text for Static segments and the parameter name otherwise.
	Value string
	// Matcher is the optional matcher name, from "id.param.integer".
	Matcher string
}

// Pattern returns the segment in URL pattern notation: "blog", "[slug]",
// "[id=integer]", "[[lang]]" or "[...path]".
func (s Segment) Pattern() string {
	m := ""
	if s.Matcher != "" {
		m = "=" + s.Matcher
	}
	switch s.Kind {
	case Param:
		return "[" + s.Value + m + "]"
	case OptionalParam:
		return "[[" + s.Value + m + "]]"
	case Rest:
		return "[..." + s.Value + m + "]"
	default:
		return s.Value
	}
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var kindSuffixes = map[string]Kind{
	"param":    Param,
	"optional": OptionalParam,
	"rest":     Rest,
}

// parseDirName parses a route directory name. It reports group=true for
// groups such as "app.group", which organize routes without adding a URL
// segment.
//
//	about              static segment
//	slug.param         required parameter        (SvelteKit: [slug])
//	id.param.integer   parameter with a matcher  (SvelteKit: [id=integer])
//	lang.optional      optional parameter        (SvelteKit: [[lang]])
//	path.rest          rest parameter            (SvelteKit: [...path])
//	app.group          group                     (SvelteKit: (app))
func parseDirName(name string) (seg Segment, group bool, err error) {
	if strings.ContainsAny(name, "[]()") {
		return Segment{}, false, legacyNameError(name)
	}

	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return Segment{Raw: name, Kind: Static, Value: name}, false, nil
	}
	kindName := parts[1]
	kind, isParam := kindSuffixes[kindName]
	if !isParam && kindName != "group" {
		// Not a recognized suffix: a static name with dots, like "v1.2".
		return Segment{Raw: name, Kind: Static, Value: name}, false, nil
	}

	ident := parts[0]
	if !identRe.MatchString(ident) {
		return Segment{}, false, fmt.Errorf("invalid directory name %q: %q is not a valid %s name (use letters, digits and _)", name, ident, kindName)
	}

	if kindName == "group" {
		if len(parts) > 2 {
			return Segment{}, false, fmt.Errorf("invalid directory name %q: groups cannot have a matcher", name)
		}
		return Segment{}, true, nil
	}

	seg = Segment{Raw: name, Kind: kind, Value: ident}
	switch len(parts) {
	case 2:
	case 3:
		if !identRe.MatchString(parts[2]) {
			return Segment{}, false, fmt.Errorf("invalid directory name %q: %q is not a valid matcher name", name, parts[2])
		}
		seg.Matcher = parts[2]
	default:
		return Segment{}, false, fmt.Errorf("invalid directory name %q: expected name.%s or name.%s.matcher", name, kindName, kindName)
	}
	return seg, false, nil
}

var (
	legacyOptional = regexp.MustCompile(`^\[\[([A-Za-z_][A-Za-z0-9_]*)(?:=([A-Za-z_][A-Za-z0-9_]*))?\]\]$`)
	legacyRest     = regexp.MustCompile(`^\[\.\.\.([A-Za-z_][A-Za-z0-9_]*)(?:=([A-Za-z_][A-Za-z0-9_]*))?\]$`)
	legacyParam    = regexp.MustCompile(`^\[([A-Za-z_][A-Za-z0-9_]*)(?:=([A-Za-z_][A-Za-z0-9_]*))?\]$`)
	legacyGroup    = regexp.MustCompile(`^\(([A-Za-z_][A-Za-z0-9_]*)\)$`)
)

// legacyNameError explains how to write a SvelteKit-style directory name in
// Tern's syntax. Go import paths cannot contain brackets or parentheses, so
// Tern uses suffixes instead.
func legacyNameError(name string) error {
	suggest := func(m []string, kind string) string {
		s := m[1] + "." + kind
		if len(m) > 2 && m[2] != "" {
			s += "." + m[2]
		}
		return s
	}
	var suggestion string
	switch {
	case legacyOptional.MatchString(name):
		suggestion = suggest(legacyOptional.FindStringSubmatch(name), "optional")
	case legacyRest.MatchString(name):
		suggestion = suggest(legacyRest.FindStringSubmatch(name), "rest")
	case legacyParam.MatchString(name):
		suggestion = suggest(legacyParam.FindStringSubmatch(name), "param")
	case legacyGroup.MatchString(name):
		suggestion = legacyGroup.FindStringSubmatch(name)[1] + ".group"
	}
	if suggestion != "" {
		return fmt.Errorf("invalid directory name %q: Go import paths cannot contain brackets or parentheses; rename it to %q", name, suggestion)
	}
	return fmt.Errorf("invalid directory name %q: Go import paths cannot contain brackets or parentheses, and segments mixing text and parameters are not supported (use an endpoint instead)", name)
}

// goSafeDir reports why a directory name cannot hold a Go package, or ""
// if it can.
func goSafeDir(name string) string {
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_'
		if !ok {
			return fmt.Sprintf("Go packages cannot live in a directory whose name contains %q", r)
		}
	}
	switch name[0] {
	case '.', '-':
		return fmt.Sprintf("Go packages cannot live in a directory whose name starts with %q", name[0])
	case '_':
		return "Go ignores directories starting with _ in ./... patterns, so tests and vet would skip this route"
	}
	if strings.HasSuffix(name, ".") {
		return "Go packages cannot live in a directory whose name ends with a dot"
	}
	return ""
}

// pathEscaper re-escapes the two characters that carry meaning in a
// normalized path, so a decoded "/" inside a segment stays distinguishable.
var pathEscaper = strings.NewReplacer("%", "%25", "/", "%2F")

// pattern returns the regular expression fragment matching this segment,
// including its leading slash.
func (s Segment) pattern() string {
	switch s.Kind {
	case Param:
		return `/([^/]+)`
	case OptionalParam:
		return `(?:/([^/]+))?`
	case Rest:
		// One or more non-empty segments: "//x" has an empty one and does
		// not match, so a rest value never starts or ends with a slash.
		return `(?:/([^/]+(?:/[^/]+)*))?`
	default:
		return "/" + regexp.QuoteMeta(pathEscaper.Replace(s.Value))
	}
}

// canonical returns the segment with parameter names removed, used to detect
// routes that would match the same URLs.
func (s Segment) canonical() string {
	switch s.Kind {
	case Static:
		return s.Value
	default:
		return fmt.Sprintf("%s:%s", s.Kind, s.Matcher)
	}
}

// Segment ranks, most specific first. rankMissing is used when a route has
// no segment at a position: it loses to static text ("/files/[...p]/edit"
// beats "/files/[...p]") but beats any parameter ("/" beats "/[[lang]]").
const (
	rankStatic = iota
	rankMissing
	rankParam // +1 without a matcher
	_
	rankOptional // +1 without a matcher
	_
	rankRest // +1 without a matcher
)

// rank orders segments by specificity; lower ranks are matched first.
func (s Segment) rank() int {
	noMatcher := 0
	if s.Matcher == "" {
		noMatcher = 1
	}
	switch s.Kind {
	case Param:
		return rankParam + noMatcher
	case OptionalParam:
		return rankOptional + noMatcher
	case Rest:
		return rankRest + noMatcher
	default:
		return rankStatic
	}
}
