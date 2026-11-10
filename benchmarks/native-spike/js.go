// Package nativespike is a spike: the Go a compiler would generate from
// Svelte components, to measure native rendering against QuickJS and check
// that the HTML matches Svelte's byte for byte. See README.md.
package nativespike

import (
	"math"
	"strconv"
	"strings"
)

// The runtime a compiler's output would call: Svelte's server helpers, and
// the parts of JavaScript's semantics templates depend on.

// escapeHTML is Svelte's escape_html: & and < in text, and " too in attributes.
func escapeHTML(b []byte, s string, attr bool) []byte {
	last := 0
	for i := 0; i < len(s); i++ {
		var rep string
		switch s[i] {
		case '&':
			rep = "&amp;"
		case '<':
			rep = "&lt;"
		case '"':
			if !attr {
				continue
			}
			rep = "&quot;"
		default:
			continue
		}
		b = append(b, s[last:i]...)
		b = append(b, rep...)
		last = i + 1
	}
	return append(b, s[last:]...)
}

// jsNumber is JavaScript's Number.prototype.toString for finite numbers.
func jsNumber(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	case x == 0:
		return "0" // including -0
	}
	// Shortest digits that round-trip, as both languages choose them.
	digits := strconv.FormatFloat(math.Abs(x), 'e', -1, 64) // d.ddde±xx
	mant, expPart, _ := strings.Cut(digits, "e")
	exp, _ := strconv.Atoi(expPart)
	ds := strings.Replace(mant, ".", "", 1)
	k := len(ds)
	n := exp + 1 // position of the decimal point
	var out string
	switch {
	case k <= n && n <= 21:
		out = ds + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		out = ds[:n] + "." + ds[n:]
	case -6 < n && n <= 0:
		out = "0." + strings.Repeat("0", -n) + ds
	default:
		e := n - 1
		sign := "+"
		if e < 0 {
			sign = "-"
			e = -e
		}
		if k == 1 {
			out = ds + "e" + sign + strconv.Itoa(e)
		} else {
			out = ds[:1] + "." + ds[1:] + "e" + sign + strconv.Itoa(e)
		}
	}
	if x < 0 {
		return "-" + out
	}
	return out
}

// jsToFixed is JavaScript's Number.prototype.toFixed: it rounds the exact
// binary value, and a true tie away from zero, unlike Go's half-to-even.
func jsToFixed(x float64, d int) string {
	if math.IsNaN(x) || math.Abs(x) >= 1e21 {
		return jsNumber(x)
	}
	neg := x < 0
	exact := strconv.FormatFloat(math.Abs(x), 'f', 60, 64) // exact for any value toFixed sees
	intPart, frac, _ := strings.Cut(exact, ".")
	keep := []byte(intPart + frac[:d])
	if frac[d] >= '5' { // ties go up: the digits after the 5 don't matter for "pick the larger n"
		i := len(keep) - 1
		for ; i >= 0; i-- {
			if keep[i] == '9' {
				keep[i] = '0'
				continue
			}
			keep[i]++
			break
		}
		if i < 0 {
			keep = append([]byte{'1'}, keep...)
		}
	}
	whole := string(keep[:len(keep)-d])
	s := whole
	if d > 0 {
		s += "." + string(keep[len(keep)-d:])
	}
	if neg { // JavaScript keeps the sign: (-0.004).toFixed(2) is "-0.00"
		s = "-" + s
	}
	return s
}
