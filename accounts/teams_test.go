package accounts

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Unfield/Tern/internal/dbtest"
)

// signUp makes an account in a browser of its own.
func (h *harness) signUp(email string) (*http.Client, int64) {
	h.t.Helper()
	c := h.client()
	status, out := h.post(c, "/signup", "email", email, "name", strings.Split(email, "@")[0], "password", "long enough password")
	if status != 200 {
		h.t.Fatalf("sign-up %s: %d %v", email, status, out)
	}
	return c, int64(out["id"].(float64))
}

func (h *harness) ok(c *http.Client, path string, kv ...string) map[string]any {
	h.t.Helper()
	status, out := h.post(c, path, kv...)
	if status != 200 {
		h.t.Fatalf("%s: %d %v", path, status, out)
	}
	return out
}

func (h *harness) fails(c *http.Client, want error, path string, kv ...string) {
	h.t.Helper()
	status, out := h.post(c, path, kv...)
	if status != 400 || out["error"] != want.Error() {
		h.t.Fatalf("%s: want %v, got %d %v", path, want, status, out)
	}
}

func roles(list []any) map[string]string {
	out := map[string]string{}
	for _, m := range list {
		m := m.(map[string]any)
		out[m["email"].(string)] = m["role"].(string)
	}
	return out
}

func TestTeams(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) { testTeams(t, db) })
}

func testTeams(t *testing.T, db *sql.DB) {
	h := newHarnessDB(t, db, func(o *Options) { o.TeamRoles = []string{"viewer"} })
	ada, adaID := h.signUp("ada@example.com")
	bob, bobID := h.signUp("bob@example.com")
	eve, _ := h.signUp("eve@example.com")

	// Without a team, RequireTeam sends users to make one.
	if status, out := h.post(ada, "/teams/current"); status != 400 || !strings.Contains(fmt.Sprint(out), "/teams/new") {
		t.Fatalf("no team: %d %v", status, out)
	}
	team := h.ok(ada, "/teams/create", "name", " Acme ")
	if team["role"] != RoleOwner || team["team"].(map[string]any)["name"] != "Acme" {
		t.Fatalf("created %v", team)
	}
	teamID := fmt.Sprint(team["team"].(map[string]any)["id"])

	// Inviting: only to roles there are; the link goes to InvitePath.
	h.fails(ada, ErrUnknownRole, "/invite", "email", "bob@example.com", "role", "boss")
	h.ok(ada, "/invite", "email", "Bob@Example.com", "role", RoleAdmin)
	e := h.lastEmail()
	if e.Kind != "invite" || e.To != "bob@example.com" || !strings.Contains(e.Link, "/invitation?token=") || !strings.Contains(e.Subject, "Acme") {
		t.Fatalf("email %+v", e)
	}
	token := tokenIn(t, e)
	if inv := h.ok(eve, "/invitation", "token", token); inv["teamName"] != "Acme" || inv["invitedBy"] != "ada" {
		t.Fatalf("invitation %v", inv)
	}
	// Only the invited address accepts.
	h.fails(eve, ErrWrongInvitee, "/invitation/accept", "token", token)
	h.fails(eve, ErrNoTeam, "/members")
	joined := h.ok(bob, "/invitation/accept", "token", token)
	if joined["role"] != RoleAdmin || fmt.Sprint(joined["team"].(map[string]any)["id"]) != teamID {
		t.Fatalf("joined %v", joined)
	}
	if me := h.ok(bob, "/me"); me["emailVerified"] != true {
		t.Error("accepting the emailed link didn't verify the address")
	}
	h.fails(bob, ErrInvalidToken, "/invitation/accept", "token", token)
	if got := roles(h.ok(bob, "/members")["list"].([]any)); got["ada@example.com"] != RoleOwner || got["bob@example.com"] != RoleAdmin {
		t.Fatalf("members %v", got)
	}
	h.fails(ada, ErrAlreadyMember, "/invite", "email", "bob@example.com", "role", RoleMember)

	// Admins manage, but not owners.
	h.fails(bob, ErrNotAllowed, "/invite", "email", "x@example.com", "role", RoleOwner)
	h.fails(bob, ErrNotAllowed, "/role", "user", fmt.Sprint(adaID), "role", RoleMember)
	h.fails(bob, ErrNotAllowed, "/remove", "user", fmt.Sprint(adaID))
	h.fails(bob, ErrNotAllowed, "/teams/delete")
	h.ok(bob, "/invite", "email", "eve@example.com", "role", "viewer")
	evetoken := tokenIn(t, h.lastEmail())
	invs := h.ok(ada, "/invitations")["list"].([]any)
	if len(invs) != 1 || invs[0].(map[string]any)["email"] != "eve@example.com" {
		t.Fatalf("invitations %v", invs)
	}
	h.ok(ada, "/invitation/cancel", "id", invs[0].(map[string]any)["id"].(string))
	h.fails(eve, ErrInvalidToken, "/invitation/accept", "token", evetoken)

	// The last owner stays one, and can't leave.
	h.fails(ada, ErrLastOwner, "/role", "user", fmt.Sprint(adaID), "role", RoleAdmin)
	h.fails(ada, ErrLastOwner, "/leave")
	h.ok(ada, "/role", "user", fmt.Sprint(bobID), "role", RoleOwner)
	h.ok(ada, "/role", "user", fmt.Sprint(adaID), "role", RoleMember)
	if got := roles(h.ok(ada, "/members")["list"].([]any)); got["ada@example.com"] != RoleMember || got["bob@example.com"] != RoleOwner {
		t.Fatalf("after the handover %v", got)
	}
	h.fails(ada, ErrNotAllowed, "/teams/rename", "name", "Mine")

	// Several teams: the session remembers which one is current.
	other := h.ok(ada, "/teams/create", "name", "Side project")
	otherID := fmt.Sprint(other["team"].(map[string]any)["id"])
	if cur := h.ok(ada, "/teams/current"); fmt.Sprint(cur["team"].(map[string]any)["id"]) != otherID {
		t.Fatalf("current after create %v", cur)
	}
	if list := h.ok(ada, "/teams")["list"].([]any); len(list) != 2 {
		t.Fatalf("teams %v", list)
	}
	h.ok(ada, "/teams/switch", "id", teamID)
	if cur := h.ok(ada, "/teams/current"); fmt.Sprint(cur["team"].(map[string]any)["id"]) != teamID {
		t.Fatalf("after switching %v", cur)
	}
	h.fails(eve, ErrNotMember, "/teams/switch", "id", teamID)

	// Removed members lose the team; it's no longer their current one.
	h.ok(bob, "/remove", "user", fmt.Sprint(adaID))
	if cur := h.ok(ada, "/teams/current"); fmt.Sprint(cur["team"].(map[string]any)["id"]) != otherID {
		t.Fatalf("after removal %v", cur)
	}

	// Deleting a team runs OnDeleteTeam; deleting the last member's
	// account deletes their team.
	h.ok(ada, "/teams/delete")
	if list := h.ok(ada, "/teams")["list"]; list != nil && len(list.([]any)) != 0 {
		t.Fatalf("teams after delete %v", list)
	}
	h.ok(bob, "/delete")
	var n int
	if err := h.acc.db.QueryRow(`SELECT COUNT(*) FROM tern_teams`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("teams left: %d %v", n, err)
	}
}

func TestDeletingTheLastOwner(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) { testDeletingTheLastOwner(t, db) })
}

func testDeletingTheLastOwner(t *testing.T, db *sql.DB) {
	var deleted []int64
	h := newHarnessDB(t, db, func(o *Options) {
		o.OnDeleteTeam = func(_ context.Context, id int64) error { deleted = append(deleted, id); return nil }
	})
	ada, _ := h.signUp("ada@example.com")
	bob, _ := h.signUp("bob@example.com")
	h.ok(ada, "/teams/create", "name", "Acme")
	h.ok(ada, "/invite", "email", "bob@example.com", "role", RoleMember)
	h.ok(bob, "/invitation/accept", "token", tokenIn(t, h.lastEmail()))
	// Ada owns a team with others in it: hand it over first.
	h.fails(ada, ErrLastOwner, "/delete")
	solo := h.ok(bob, "/teams/create", "name", "Solo")
	h.ok(bob, "/delete")
	if want := int64(solo["team"].(map[string]any)["id"].(float64)); !slices.Equal(deleted, []int64{want}) {
		t.Fatalf("OnDeleteTeam ran for %v, want [%d]", deleted, want)
	}
	if got := roles(h.ok(ada, "/members")["list"].([]any)); len(got) != 1 {
		t.Fatalf("bob is still a member: %v", got)
	}
}

func TestAuditLog(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) { testAuditLog(t, db) })
}

func testAuditLog(t *testing.T, db *sql.DB) {
	h := newHarnessDB(t, db)
	ada, _ := h.signUp("ada@example.com")
	other := h.client()
	h.post(other, "/signin", "email", "ada@example.com", "password", "wrong password")
	h.ok(ada, "/password", "current", "long enough password", "password", "another long password")
	h.ok(ada, "/teams/create", "name", "Acme")
	h.ok(ada, "/invite", "email", "bob@example.com", "role", RoleMember)

	actions := func(list []any) []string {
		var out []string
		for _, e := range list {
			out = append(out, e.(map[string]any)["action"].(string))
		}
		return out
	}
	mine := h.ok(ada, "/activity")["list"].([]any)
	want := []string{"member.invited", "team.created", "user.password_changed", "user.sign_in_failed", "user.signed_in", "user.signed_up"}
	if got := actions(mine); !slices.Equal(got, want) {
		t.Fatalf("activity %v, want %v", got, want)
	}
	first := mine[0].(map[string]any)
	if first["target"] != "bob@example.com" || first["actor"] != "ada@example.com" || first["device"] != "Firefox on macOS" || first["data"].(map[string]any)["role"] != RoleMember {
		t.Fatalf("entry %v", first)
	}
	// Paging back.
	older := h.ok(ada, "/activity", "before", fmt.Sprint(mine[1].(map[string]any)["id"]))["list"].([]any)
	if got := actions(older); !slices.Equal(got, want[2:]) {
		t.Fatalf("older %v", got)
	}
	if got := actions(h.ok(ada, "/team/activity")["list"].([]any)); !slices.Equal(got, []string{"member.invited", "team.created"}) {
		t.Fatalf("team activity %v", got)
	}
	entries, err := h.acc.AuditEntries(context.Background(), AuditFilter{Action: "user.sign"})
	if err != nil || len(entries) != 3 {
		t.Fatalf("filtered %v %v", entries, err)
	}
}
