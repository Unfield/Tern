package js

import (
	"strings"
	"testing"
)

// Sticky and global exec at a UTF-16 index, after non-ASCII text, in
// strings long and short.
func TestRegExpIndexAfterNonASCII(t *testing.T) {
	rt := NewRealm()
	lone := FromUnits([]uint16{0xD800})
	for _, c := range []struct {
		pre string
		at  int // where <b> starts, in UTF-16 units
	}{
		{"·x · ", 5}, {strings.Repeat("a", 40) + "·x · ", 45}, {strings.Repeat("a", 200) + "·x · ", 205},
		{"😀 é ", 5}, {"a" + lone + "b", 3},
	} {
		s := c.pre + "<b>"
		at, pad := c.at, len(c.pre)
		for _, flags := range []string{"y", "g"} {
			re := rt.NewRegExp(`<b>`, flags)
			Set(re, "lastIndex", float64(at))
			m := Call(rt.Get(re, "exec"), re, s)
			if m == Null {
				t.Errorf("pad %d /%s: no match at %d", pad, flags, at)
				continue
			}
			if got := ToNumber(rt.Get(m, "index")); int(got) != at {
				t.Errorf("pad %d /%s: index %v, want %d", pad, flags, got, at)
			}
			if got := ToNumber(rt.Get(re, "lastIndex")); int(got) != at+3 {
				t.Errorf("pad %d /%s: lastIndex %v, want %d", pad, flags, got, at+3)
			}
		}
	}
}
