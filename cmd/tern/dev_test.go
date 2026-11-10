package main

import (
	"bytes"
	"slices"
	"testing"

	"github.com/Unfield/Tern/internal/routes"
	"github.com/Unfield/Tern/internal/ui"
)

func TestDiffRoutes(t *testing.T) {
	tests := []struct {
		name    string
		prev    []string
		next    []string
		added   []string
		removed []string
	}{
		{
			name:    "empty to routes",
			prev:    nil,
			next:    []string{"/", "/about"},
			added:   []string{"/", "/about"},
			removed: nil,
		},
		{
			name:    "no change",
			prev:    []string{"/", "/about"},
			next:    []string{"/", "/about"},
			added:   nil,
			removed: nil,
		},
		{
			name:    "add route",
			prev:    []string{"/", "/about"},
			next:    []string{"/", "/about", "/projects/[id]"},
			added:   []string{"/projects/[id]"},
			removed: nil,
		},
		{
			name:    "remove route",
			prev:    []string{"/", "/about", "/old-page"},
			next:    []string{"/", "/about"},
			added:   nil,
			removed: []string{"/old-page"},
		},
		{
			name:    "add and remove routes",
			prev:    []string{"/", "/old-page"},
			next:    []string{"/", "/projects/[id]"},
			added:   []string{"/projects/[id]"},
			removed: []string{"/old-page"},
		},
		{
			name:    "alphabetical sorting",
			prev:    []string{"/z", "/b"},
			next:    []string{"/a", "/c"},
			added:   []string{"/a", "/c"},
			removed: []string{"/b", "/z"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotAdded, gotRemoved := diffRoutes(tc.prev, tc.next)
			if !slices.Equal(gotAdded, tc.added) {
				t.Errorf("diffRoutes added = %v, want %v", gotAdded, tc.added)
			}
			if !slices.Equal(gotRemoved, tc.removed) {
				t.Errorf("diffRoutes removed = %v, want %v", gotRemoved, tc.removed)
			}
		})
	}
}

func TestReportRouteChanges(t *testing.T) {
	var buf bytes.Buffer
	printer := ui.New(&buf)
	d := &devRunner{ui: printer}

	m1 := &routes.Manifest{
		Routes: []*routes.Route{
			{Segments: []routes.Segment{{Value: "old", Kind: routes.Static}}},
		},
	}
	// Initial call should only populate d.routes and not print additions
	d.reportRouteChanges(m1)
	if buf.Len() > 0 {
		t.Errorf("initial reportRouteChanges printed unexpected output: %s", buf.String())
	}
	if !slices.Equal(d.routes, []string{"/old"}) {
		t.Errorf("d.routes = %v, want ['/old']", d.routes)
	}

	// Subsequent call removing /old and adding /projects/[id]
	m2 := &routes.Manifest{
		Routes: []*routes.Route{
			{
				Segments: []routes.Segment{
					{Value: "projects", Kind: routes.Static},
					{Value: "id", Kind: routes.Param},
				},
			},
		},
	}
	d.reportRouteChanges(m2)
	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("+ route /projects/[id]")) {
		t.Errorf("expected added route in output, got: %s", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("- route /old")) {
		t.Errorf("expected removed route in output, got: %s", out)
	}
}
