package routes

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseDirName(t *testing.T) {
	tests := []struct {
		name      string
		wantParts []Part
		wantGroup bool
	}{
		{name: "blog", wantParts: []Part{{Kind: Static, Value: "blog"}}},
		{name: "(app)", wantGroup: true},
		{name: "[slug]", wantParts: []Part{{Kind: Param, Value: "slug"}}},
		{name: "[id=integer]", wantParts: []Part{{Kind: Param, Value: "id", Matcher: "integer"}}},
		{name: "[[lang]]", wantParts: []Part{{Kind: OptionalParam, Value: "lang"}}},
		{name: "[[lang=locale]]", wantParts: []Part{{Kind: OptionalParam, Value: "lang", Matcher: "locale"}}},
		{name: "[...path]", wantParts: []Part{{Kind: Rest, Value: "path"}}},
		{name: "[...path=file]", wantParts: []Part{{Kind: Rest, Value: "path", Matcher: "file"}}},
		{name: "[slug].json", wantParts: []Part{{Kind: Param, Value: "slug"}, {Kind: Static, Value: ".json"}}},
		{name: "v[major]-[minor]", wantParts: []Part{
			{Kind: Static, Value: "v"}, {Kind: Param, Value: "major"},
			{Kind: Static, Value: "-"}, {Kind: Param, Value: "minor"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg, group, err := parseDirName(tt.name)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if group != tt.wantGroup {
				t.Errorf("group = %v, want %v", group, tt.wantGroup)
			}
			if !reflect.DeepEqual(seg.Parts, tt.wantParts) {
				t.Errorf("parts = %+v, want %+v", seg.Parts, tt.wantParts)
			}
		})
	}
}

func TestParseDirNameErrors(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
	}{
		{"()", "invalid group name"},
		{"(a[b])", "invalid group name"},
		{"a(b)", "parentheses are only allowed for groups"},
		{"[slug", "unclosed '['"},
		{"[[lang]", "unclosed '[['"},
		{"slug]", "unmatched ']'"},
		{"[a]]", "unmatched ']'"},
		{"[]", "invalid parameter name"},
		{"[1st]", "invalid parameter name"},
		{"[my-slug]", "invalid parameter name"},
		{"[id=]", "invalid matcher name"},
		{"[id=is-int]", "invalid matcher name"},
		{"[[...rest]]", "rest parameters are already optional"},
		{"[a][b]", "must be separated by static text"},
		{"x[[lang]]", "optional parameters must be the whole directory name"},
		{"[...path].json", "rest parameters must be the whole directory name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseDirName(tt.name)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
