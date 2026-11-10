// Package rbac checks permissions by role:
//
//	var policy = rbac.New(map[string][]string{
//		"viewer": {"projects:read"},
//		"editor": {"projects:read", "projects:write"},
//		"admin":  {"*"},
//	}, func(ctx *kit.Ctx) []string {
//		return sessions.Get(ctx).Strings("roles") // however your app stores them
//	})
//
//	func Load(ctx *kit.Ctx) (Page, error) {
//		if err := policy.Require(ctx, "projects:read"); err != nil {
//			return Page{}, err // 401 when signed out, 403 without the permission
//		}
//		...
//	}
//
// Permissions are strings; "projects:*" grants every permission that
// starts with "projects:", and "*" grants everything. A role can include
// other roles with "role:<name>".
package rbac

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/Unfield/Tern/kit"
)

// Policy maps roles to permissions.
type Policy struct {
	roles map[string][]string
	of    func(*kit.Ctx) []string
}

// New returns a policy. roles maps each role to its permissions; rolesOf
// returns the current user's roles, or nil when nobody is signed in.
func New(roles map[string][]string, rolesOf func(*kit.Ctx) []string) *Policy {
	p := &Policy{roles: map[string][]string{}, of: rolesOf}
	for r, perms := range roles {
		p.roles[r] = slices.Clone(perms)
	}
	return p
}

// Permissions returns everything the roles grant, including included roles.
func (p *Policy) Permissions(roles ...string) []string {
	var out []string
	seen := map[string]bool{}
	var walk func(r string)
	walk = func(r string) {
		if seen[r] {
			return
		}
		seen[r] = true
		for _, perm := range p.roles[r] {
			if inc, ok := strings.CutPrefix(perm, "role:"); ok {
				walk(inc)
			} else if !slices.Contains(out, perm) {
				out = append(out, perm)
			}
		}
	}
	for _, r := range roles {
		walk(r)
	}
	return out
}

// Allows reports whether any of roles grants perm.
func (p *Policy) Allows(roles []string, perm string) bool {
	for _, g := range p.Permissions(roles...) {
		if match(g, perm) {
			return true
		}
	}
	return false
}

func match(grant, perm string) bool {
	if grant == "*" || grant == perm {
		return true
	}
	prefix, ok := strings.CutSuffix(grant, "*")
	return ok && strings.HasPrefix(perm, prefix)
}

// Can reports whether the current user has perm.
func (p *Policy) Can(ctx *kit.Ctx, perm string) bool { return p.Allows(p.of(ctx), perm) }

// Require returns nil if the current user has every perm, a 401 error if
// nobody is signed in, and a 403 error otherwise.
func (p *Policy) Require(ctx *kit.Ctx, perms ...string) error {
	roles := p.of(ctx)
	if roles == nil {
		return kit.Error(http.StatusUnauthorized, "Sign in to continue")
	}
	for _, perm := range perms {
		if !p.Allows(roles, perm) {
			return kit.Error(http.StatusForbidden, fmt.Sprintf("You don't have permission to %s", perm))
		}
	}
	return nil
}

// Hook returns a kit hook that requires perm for every request whose path
// starts with prefix, such as rbac.Hook("/admin", "admin:access").
func (p *Policy) Hook(prefix, perm string) kit.HookFunc {
	return func(ctx *kit.Ctx) error {
		path := ctx.Request.URL.Path
		if path != prefix && !strings.HasPrefix(path, strings.TrimSuffix(prefix, "/")+"/") {
			return nil
		}
		return p.Require(ctx, perm)
	}
}
