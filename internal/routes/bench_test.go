package routes

import (
	"fmt"
	"testing"
)

// BenchmarkMatch matches against 200 routes of mixed kinds.
func BenchmarkMatch(b *testing.B) {
	var rs []*Route
	for i := range 50 {
		for _, id := range []string{"/section%d", "/section%d/slug.param", "/section%d/id.param.integer/edit", "/docs%d/path.rest"} {
			segs, err := ParseID(fmt.Sprintf(id, i))
			if err != nil {
				b.Fatal(err)
			}
			rs = append(rs, &Route{ID: fmt.Sprintf(id, i), Segments: segs})
		}
	}
	Sort(rs)
	r, err := NewRouter(rs, map[string]Matcher{"integer": func(string) bool { return true }})
	if err != nil {
		b.Fatal(err)
	}
	paths := []string{"/section25", "/section40/hello-world", "/section10/42/edit", "/docs49/a/b/c", "/nope/nope"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		r.Match(paths[i%len(paths)])
	}
}
