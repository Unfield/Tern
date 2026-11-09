package routes

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// PartKind is the kind of a piece of a route segment.
type PartKind int

const (
	// Static is literal text, such as "blog" or ".json".
	Static PartKind = iota
	// Param is a required parameter: [slug].
	Param
	// OptionalParam is a parameter that may be absent: [[lang]].
	OptionalParam
	// Rest matches zero or more path segments: [...path].
	Rest
)

func (k PartKind) String() string {
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
		return fmt.Sprintf("PartKind(%d)", int(k))
	}
}

// Part is one piece of a segment. A segment such as "[slug].json" has two
// parts: a Param named "slug" and the Static text ".json".
type Part struct {
	Kind PartKind
	// Value is the literal text for Static parts and the parameter name otherwise.
	Value string
	// Matcher is the optional matcher name, from [id=integer].
	Matcher string
}

// Segment is one URL path segment, parsed from a route directory name.
type Segment struct {
	// Raw is the directory name, e.g. "[slug]".
	Raw   string
	Parts []Part
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseDirName parses a route directory name. It reports group=true for
// layout groups such as "(app)", which do not contribute a URL segment.
func parseDirName(name string) (seg Segment, group bool, err error) {
	if strings.HasPrefix(name, "(") && strings.HasSuffix(name, ")") {
		inner := name[1 : len(name)-1]
		if inner == "" || strings.ContainsAny(inner, "()[]") {
			return Segment{}, true, fmt.Errorf("invalid group name %q", name)
		}
		return Segment{}, true, nil
	}
	if strings.ContainsAny(name, "()") {
		return Segment{}, false, fmt.Errorf("invalid directory name %q: parentheses are only allowed for groups like (name)", name)
	}

	seg.Raw = name
	rest := name
	for rest != "" {
		i := strings.IndexByte(rest, '[')
		if i < 0 {
			if strings.ContainsRune(rest, ']') {
				return Segment{}, false, fmt.Errorf("invalid directory name %q: unmatched ']'", name)
			}
			seg.Parts = append(seg.Parts, Part{Kind: Static, Value: rest})
			break
		}
		if i > 0 {
			text := rest[:i]
			if strings.ContainsRune(text, ']') {
				return Segment{}, false, fmt.Errorf("invalid directory name %q: unmatched ']'", name)
			}
			seg.Parts = append(seg.Parts, Part{Kind: Static, Value: text})
			rest = rest[i:]
		}

		p, n, err := parseParam(rest)
		if err != nil {
			return Segment{}, false, fmt.Errorf("invalid directory name %q: %w", name, err)
		}
		if k := len(seg.Parts); k > 0 && seg.Parts[k-1].Kind != Static {
			return Segment{}, false, fmt.Errorf("invalid directory name %q: parameters must be separated by static text", name)
		}
		seg.Parts = append(seg.Parts, p)
		rest = rest[n:]
	}

	if len(seg.Parts) > 1 {
		for _, p := range seg.Parts {
			if p.Kind == OptionalParam || p.Kind == Rest {
				return Segment{}, false, fmt.Errorf("invalid directory name %q: %s parameters must be the whole directory name", name, p.Kind)
			}
		}
	}
	return seg, false, nil
}

// parseParam parses the parameter at the start of s, which begins with '['.
// It returns the part and the number of bytes consumed.
func parseParam(s string) (Part, int, error) {
	optional := strings.HasPrefix(s, "[[")
	var inner string
	var n int
	if optional {
		end := strings.Index(s, "]]")
		if end < 0 {
			return Part{}, 0, errors.New("unclosed '[['")
		}
		inner, n = s[2:end], end+2
	} else {
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return Part{}, 0, errors.New("unclosed '['")
		}
		inner, n = s[1:end], end+1
	}
	if strings.ContainsAny(inner, "[]") {
		return Part{}, 0, errors.New("nested brackets")
	}

	kind := Param
	if optional {
		kind = OptionalParam
	}
	if after, ok := strings.CutPrefix(inner, "..."); ok {
		if optional {
			return Part{}, 0, errors.New("rest parameters are already optional; use [...name]")
		}
		kind, inner = Rest, after
	}

	name, matcher, hasMatcher := strings.Cut(inner, "=")
	if !identRe.MatchString(name) {
		return Part{}, 0, fmt.Errorf("invalid parameter name %q", name)
	}
	if hasMatcher && !identRe.MatchString(matcher) {
		return Part{}, 0, fmt.Errorf("invalid matcher name %q", matcher)
	}
	return Part{Kind: kind, Value: name, Matcher: matcher}, n, nil
}

// pathEscaper re-escapes the two characters that carry meaning in a
// normalized path, so a decoded "/" inside a segment stays distinguishable.
var pathEscaper = strings.NewReplacer("%", "%25", "/", "%2F")

// pattern returns the regular expression fragment matching this segment,
// including its leading slash.
func (s Segment) pattern() string {
	if len(s.Parts) == 1 {
		p := s.Parts[0]
		switch p.Kind {
		case Static:
			return "/" + regexp.QuoteMeta(pathEscaper.Replace(p.Value))
		case Param:
			return `/([^/]+)`
		case OptionalParam:
			return `(?:/([^/]+))?`
		case Rest:
			return `(?:/(.+))?`
		}
	}
	var b strings.Builder
	b.WriteByte('/')
	for _, p := range s.Parts {
		if p.Kind == Static {
			b.WriteString(regexp.QuoteMeta(pathEscaper.Replace(p.Value)))
		} else {
			b.WriteString(`([^/]+?)`)
		}
	}
	return b.String()
}

// canonical returns the segment with parameter names removed, used to detect
// routes that would match the same URLs.
func (s Segment) canonical() string {
	var b strings.Builder
	for _, p := range s.Parts {
		switch p.Kind {
		case Static:
			b.WriteString(p.Value)
		case Param:
			b.WriteString("[=" + p.Matcher + "]")
		case OptionalParam:
			b.WriteString("[[=" + p.Matcher + "]]")
		case Rest:
			b.WriteString("[...=" + p.Matcher + "]")
		}
	}
	return b.String()
}

// Segment ranks, most specific first. rankMissing is used when a route has
// no segment at a position: it loses to static text ("/files/[...p]/edit"
// beats "/files/[...p]") but beats any parameter ("/" beats "/[[lang]]").
const (
	rankStatic = iota
	rankMissing
	rankMixed
	rankParam // +1 without a matcher
	_
	rankOptional // +1 without a matcher
	_
	rankRest // +1 without a matcher
)

// rank orders segments by specificity; lower ranks are matched first.
func (s Segment) rank() int {
	if len(s.Parts) > 1 {
		return rankMixed // e.g. [slug].json
	}
	p := s.Parts[0]
	noMatcher := 0
	if p.Matcher == "" {
		noMatcher = 1
	}
	switch p.Kind {
	case Static:
		return rankStatic
	case Param:
		return rankParam + noMatcher
	case OptionalParam:
		return rankOptional + noMatcher
	default: // Rest
		return rankRest + noMatcher
	}
}
