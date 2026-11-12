package js

import (
	"fmt"
	"strings"
	"unicode"
)

// The Unicode properties ID_Start and ID_Continue, which regexp2 doesn't
// know, as class contents it does: the general categories they're made of
// and the characters Unicode adds to them (Other_ID_Start and
// Other_ID_Continue). Pattern_Syntax and Pattern_White_Space, which the
// definitions take out, share no characters with these.
var (
	idStartClass    = `\p{L}\p{Nl}` + rangesClass(unicode.Other_ID_Start)
	idContinueClass = idStartClass + `\p{Mn}\p{Mc}\p{Nd}\p{Pc}` + rangesClass(unicode.Other_ID_Continue)
)

func rangesClass(t *unicode.RangeTable) string {
	var b strings.Builder
	esc := func(r rune) string {
		if r > 0xFFFF {
			return fmt.Sprintf(`\u{%X}`, r)
		}
		return fmt.Sprintf(`\u%04X`, r)
	}
	for _, r := range t.R16 {
		for lo := rune(r.Lo); lo <= rune(r.Hi); lo += rune(r.Stride) {
			b.WriteString(esc(lo))
			if r.Stride == 1 {
				if r.Hi > r.Lo {
					b.WriteString("-" + esc(rune(r.Hi)))
				}
				break
			}
		}
	}
	for _, r := range t.R32 {
		for lo := rune(r.Lo); lo <= rune(r.Hi); lo += rune(r.Stride) {
			b.WriteString(esc(lo))
			if r.Stride == 1 {
				if r.Hi > r.Lo {
					b.WriteString("-" + esc(rune(r.Hi)))
				}
				break
			}
		}
	}
	return b.String()
}

// expandIDProps replaces \p{ID_Start} and \p{ID_Continue}, and their \P
// negations outside classes, with what regexp2 can compile. ok is false
// for a negation inside a class, which would need class subtraction.
func expandIDProps(source string) (out string, ok bool) {
	if !strings.Contains(source, "{ID_") {
		return source, true
	}
	var b strings.Builder
	inClass := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '\\' && i+1 < len(source) {
			if (source[i+1] == 'p' || source[i+1] == 'P') && i+2 < len(source) && source[i+2] == '{' {
				end := strings.IndexByte(source[i:], '}')
				if end > 0 {
					name := source[i+3 : i+end]
					class := ""
					switch name {
					case "ID_Start":
						class = idStartClass
					case "ID_Continue":
						class = idContinueClass
					}
					if class != "" {
						neg := source[i+1] == 'P'
						switch {
						case inClass && neg:
							return "", false
						case inClass:
							b.WriteString(class)
						case neg:
							b.WriteString("[^" + class + "]")
						default:
							b.WriteString("[" + class + "]")
						}
						i += end
						continue
					}
				}
			}
			b.WriteByte(c)
			b.WriteByte(source[i+1])
			i++
			continue
		}
		switch c {
		case '[':
			inClass = true
		case ']':
			inClass = false
		}
		b.WriteByte(c)
	}
	return b.String(), true
}
