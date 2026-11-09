package routes

import (
	"strings"
	"testing"
)

func TestParseDirName(t *testing.T) {
	tests := []struct {
		name      string
		want      Segment
		wantGroup bool
	}{
		{name: "blog", want: Segment{Kind: Static, Value: "blog"}},
		{name: "v1.2", want: Segment{Kind: Static, Value: "v1.2"}},
		{name: "sitemap.xml", want: Segment{Kind: Static, Value: "sitemap.xml"}},
		{name: "app.group", wantGroup: true},
		{name: "slug.param", want: Segment{Kind: Param, Value: "slug"}},
		{name: "id.param.integer", want: Segment{Kind: Param, Value: "id", Matcher: "integer"}},
		{name: "lang.optional", want: Segment{Kind: OptionalParam, Value: "lang"}},
		{name: "lang.optional.locale", want: Segment{Kind: OptionalParam, Value: "lang", Matcher: "locale"}},
		{name: "path.rest", want: Segment{Kind: Rest, Value: "path"}},
		{name: "path.rest.file", want: Segment{Kind: Rest, Value: "path", Matcher: "file"}},
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
			if tt.wantGroup {
				return
			}
			tt.want.Raw = tt.name
			if seg != tt.want {
				t.Errorf("segment = %+v, want %+v", seg, tt.want)
			}
		})
	}
}

func TestParseDirNameErrors(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
	}{
		{"[slug]", `rename it to "slug.param"`},
		{"[id=integer]", `rename it to "id.param.integer"`},
		{"[[lang]]", `rename it to "lang.optional"`},
		{"[...path]", `rename it to "path.rest"`},
		{"(app)", `rename it to "app.group"`},
		{"[slug].json", "segments mixing text and parameters are not supported"},
		{"my-slug.param", `"my-slug" is not a valid param name`},
		{"1st.param", `"1st" is not a valid param name`},
		{"id.param.is-int", `"is-int" is not a valid matcher name`},
		{"id.param.a.b", "expected name.param or name.param.matcher"},
		{"app.group.x", "groups cannot have a matcher"},
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

func TestGoSafeDir(t *testing.T) {
	for _, ok := range []string{"blog", "slug.param", "my-page", "my_page", "v1.2"} {
		if reason := goSafeDir(ok); reason != "" {
			t.Errorf("goSafeDir(%q) = %q, want safe", ok, reason)
		}
	}
	for _, bad := range []string{"über", "-x", ".x", "_x", "x.", "a b", "a~b", "a+b"} {
		if goSafeDir(bad) == "" {
			t.Errorf("goSafeDir(%q) = safe, want a reason", bad)
		}
	}
}
