package kit

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Access says who may use a route: its pages, their data, its form actions
// and its endpoints. A route package declares it, and code generation reads
// it (ADR 0027):
//
//	var Access = kit.Public                   // anyone
//	var Access = kit.SignedIn                 // a signed-in user
//	var Access = kit.Permission("links.edit") // a signed-in user with the permission
//
// SignedIn and Permission apply to the directories below too, unless one
// declares its own; Public applies to its own directory only. A route with
// server code (a load, actions or an endpoint) that has no Access, of its
// own or from above, doesn't build: routes are closed by default.
type Access struct {
	level access
	perms []string
}

type access int

const (
	accessNone access = iota
	accessPublic
	accessSignedIn
	accessPermission
)

var (
	// Public lets anyone use the route.
	Public = Access{level: accessPublic}
	// SignedIn lets signed-in users use the route; see WithSignedIn.
	SignedIn = Access{level: accessSignedIn}
)

// Permission lets signed-in users with all of perms use the route; see
// WithPermissions.
func Permission(perms ...string) Access {
	return Access{level: accessPermission, perms: perms}
}

// Restricted reports whether the rule keeps anyone out.
func (a Access) Restricted() bool { return a.level >= accessSignedIn }

// String describes the rule, as in "kit.Permission(\"links.edit\")".
func (a Access) String() string {
	switch a.level {
	case accessPublic:
		return "kit.Public"
	case accessSignedIn:
		return "kit.SignedIn"
	case accessPermission:
		return fmt.Sprintf("kit.Permission(%q)", strings.Join(a.perms, `", "`))
	}
	return "no Access"
}

// WithSignedIn gives Tern the check for kit.SignedIn routes: it returns
// nil for a signed-in user, and otherwise an error such as a redirect to
// the sign-in page. accounts provides one: acc.Guard().
func WithSignedIn(check func(*Ctx) error) Option {
	return func(o *options) { o.signedIn = check }
}

// WithPermissions gives Tern the check for kit.Permission routes: it
// returns nil when the user has every one of perms, and otherwise an
// error, such as kit.Error(403, …). It runs after the WithSignedIn check.
// rbac provides one: policy.Guard().
func WithPermissions(check func(c *Ctx, perms ...string) error) Option {
	return func(o *options) { o.permissions = check }
}

// checkAccessConfig makes sure the checks the routes need are there.
func checkAccessConfig(m *Manifest, o *options) error {
	var needSignedIn, needPerms []string
	for _, def := range m.Routes {
		switch def.Access.level {
		case accessSignedIn:
			needSignedIn = append(needSignedIn, def.ID)
		case accessPermission:
			needSignedIn = append(needSignedIn, def.ID)
			needPerms = append(needPerms, def.ID)
		}
	}
	if len(needSignedIn) > 0 && o.signedIn == nil {
		return fmt.Errorf("kit: routes such as %s are for signed-in users, but nothing checks who's signed in: add kit.WithSignedIn, such as acc.Guard() from accounts", needSignedIn[0])
	}
	if len(needPerms) > 0 && o.permissions == nil {
		return fmt.Errorf("kit: routes such as %s need permissions, but nothing checks them: add kit.WithPermissions, such as policy.Guard() from rbac", needPerms[0])
	}
	return nil
}

// checkAccess applies the route's Access to a request.
func (a *App) checkAccess(c *Ctx, def *RouteDef, endpoint bool) error {
	if !def.Access.Restricted() {
		return nil
	}
	err := a.opts.signedIn(c)
	if err == nil && def.Access.level == accessPermission {
		err = a.opts.permissions(c, def.Access.perms...)
	}
	var re *RedirectError
	if endpoint && errors.As(err, &re) {
		// A program calling an endpoint can't follow a redirect to a
		// sign-in page.
		return Error(http.StatusUnauthorized, "Sign in to use this.")
	}
	return err
}
