package kit

import (
	"net/http"
	"strings"
	"testing"
)

func accessManifest() *Manifest {
	m := testManifest()
	for i := range m.Routes {
		switch m.Routes[i].ID {
		case "/todos":
			m.Routes[i].Access = SignedIn
		case "/api/items/id.param.integer":
			m.Routes[i].Access = Permission("items.delete")
		case "/feed":
			m.Routes[i].Access = SignedIn
			m.Routes[i].Revalidate = 60e9 // never cached: it's per user
		}
	}
	return m
}

func accessOptions() []Option {
	return []Option{
		WithSignedIn(func(c *Ctx) error {
			if c.Header("Authorization") == "" {
				return Redirect(http.StatusSeeOther, "/login")
			}
			return nil
		}),
		WithPermissions(func(c *Ctx, perms ...string) error {
			if c.Header("Authorization") != "admin" {
				return Error(http.StatusForbidden, "not allowed: "+strings.Join(perms, ","))
			}
			return nil
		}),
	}
}

func TestAccess(t *testing.T) {
	a, _ := newTestApp(t, accessManifest(), accessOptions()...)

	// Signed out: pages and their data redirect to the sign-in page, and
	// form actions don't run.
	if rec := do(t, a, "GET", "/todos", nil); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Errorf("page: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if p := jsonPayload(t, do(t, a, "GET", "/todos/__data.json", nil)); p.Type != "redirect" {
		t.Errorf("data: %+v", p)
	}
	if rec := do(t, a, "POST", "/todos", strings.NewReader("title=x"), "Content-Type", "application/x-www-form-urlencoded"); rec.Code != http.StatusSeeOther {
		t.Errorf("action: %d", rec.Code)
	}
	// Endpoints answer 401 instead of redirecting.
	if rec := do(t, a, "GET", "/api/items/1", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("endpoint signed out: %d", rec.Code)
	}

	// Signed in.
	if rec := do(t, a, "GET", "/todos", nil, "Authorization", "alice"); rec.Code != http.StatusOK {
		t.Errorf("page signed in: %d", rec.Code)
	}
	if rec := do(t, a, "GET", "/feed", nil, "Authorization", "alice", "Accept", "text/html"); rec.Code != http.StatusOK || rec.Header().Get("X-Tern-Cache") != "" {
		t.Errorf("a restricted page with Revalidate: %d, cache %q", rec.Code, rec.Header().Get("X-Tern-Cache"))
	}
	// Permissions.
	if rec := do(t, a, "GET", "/api/items/1", nil, "Authorization", "alice"); rec.Code != http.StatusForbidden {
		t.Errorf("endpoint without the permission: %d", rec.Code)
	}
	if rec := do(t, a, "GET", "/api/items/1", nil, "Authorization", "admin"); rec.Code != http.StatusOK {
		t.Errorf("endpoint with the permission: %d", rec.Code)
	}
	// Routes without a rule are open.
	if rec := do(t, a, "GET", "/", nil); rec.Code != http.StatusOK {
		t.Errorf("open route: %d", rec.Code)
	}
}

func TestAccessNeedsChecks(t *testing.T) {
	if _, err := New(accessManifest()); err == nil || !strings.Contains(err.Error(), "kit.WithSignedIn") {
		t.Errorf("no WithSignedIn: %v", err)
	}
	if _, err := New(accessManifest(), accessOptions()[0]); err == nil || !strings.Contains(err.Error(), "kit.WithPermissions") {
		t.Errorf("no WithPermissions: %v", err)
	}
}
