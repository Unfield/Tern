package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Unfield/Tern/kit"
)

var policy = New(map[string][]string{
	"viewer": {"projects:read"},
	"editor": {"role:viewer", "projects:write", "comments:*"},
	"admin":  {"*"},
	"loop":   {"role:loop", "x"},
}, func(ctx *kit.Ctx) []string {
	r := ctx.Request.Header.Get("X-Roles")
	if r == "" {
		return nil
	}
	return []string{r}
})

func TestAllows(t *testing.T) {
	for _, tt := range []struct {
		role, perm string
		want       bool
	}{
		{"viewer", "projects:read", true},
		{"viewer", "projects:write", false},
		{"editor", "projects:read", true}, // through role:viewer
		{"editor", "comments:delete", true},
		{"editor", "commentsX", false},
		{"admin", "anything:at:all", true},
		{"loop", "x", true}, // cycles are fine
		{"nobody", "projects:read", false},
	} {
		if got := policy.Allows([]string{tt.role}, tt.perm); got != tt.want {
			t.Errorf("%s %s = %v, want %v", tt.role, tt.perm, got, tt.want)
		}
	}
}

func TestRequireAndHook(t *testing.T) {
	m := &kit.Manifest{
		AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`,
		Routes: []kit.RouteDef{
			{ID: "/admin/users", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error { return c.Text(200, "users") }}},
			{ID: "/projects", Endpoint: map[string]kit.HandlerFunc{"POST": func(c *kit.Ctx) error {
				if err := policy.Require(c, "projects:write"); err != nil {
					return err
				}
				return c.Text(200, "created")
			}}},
			{ID: "/administrator", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error { return c.Text(200, "not admin") }}},
		},
	}
	a, err := kit.New(m, kit.WithHook(policy.Hook("/admin", "admin:access")))
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, path, role string) int {
		r := httptest.NewRequest(method, path, nil)
		if role != "" {
			r.Header.Set("X-Roles", role)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w.Code
	}
	for _, tt := range []struct {
		method, path, role string
		want               int
	}{
		{"GET", "/admin/users", "", http.StatusUnauthorized},
		{"GET", "/admin/users", "editor", http.StatusForbidden},
		{"GET", "/admin/users", "admin", http.StatusOK},
		{"GET", "/administrator", "", http.StatusOK}, // not under /admin/
		{"POST", "/projects", "viewer", http.StatusForbidden},
		{"POST", "/projects", "editor", http.StatusOK},
	} {
		if got := do(tt.method, tt.path, tt.role); got != tt.want {
			t.Errorf("%s %s as %q = %d, want %d", tt.method, tt.path, tt.role, got, tt.want)
		}
	}
}

func TestGuard(t *testing.T) {
	m := &kit.Manifest{
		AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`,
		Routes: []kit.RouteDef{
			{ID: "/projects", Access: kit.Permission("projects:write"), Endpoint: map[string]kit.HandlerFunc{"POST": func(c *kit.Ctx) error { return c.Text(200, "created") }}},
		},
	}
	signedIn := kit.WithSignedIn(func(c *kit.Ctx) error {
		if c.Header("X-Roles") == "" {
			return kit.Error(http.StatusUnauthorized, "Sign in")
		}
		return nil
	})
	a, err := kit.New(m, signedIn, policy.Guard())
	if err != nil {
		t.Fatal(err)
	}
	for role, want := range map[string]int{"": http.StatusUnauthorized, "viewer": http.StatusForbidden, "editor": http.StatusOK} {
		r := httptest.NewRequest(http.MethodPost, "/projects", nil)
		if role != "" {
			r.Header.Set("X-Roles", role)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("%q: %d, want %d", role, w.Code, want)
		}
	}
}
