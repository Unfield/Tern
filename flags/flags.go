// Package flags turns features on for some users and not others, decided
// on the server: a kill switch for each feature, lists of users and teams
// who get it first, and a rollout to a share of users, teams or visitors
// that grows without anyone losing it.
//
//	f, err := flags.New(db, flags.Options{Subject: flags.AccountsSubject},
//		flags.Flag{Name: "new-editor", Description: "The editor with live previews"},
//	)
//
//	if f.Enabled(c, "new-editor") { ... }
//
// Flags are kept in the database, so they change without a deploy, and
// read from memory, so checking one costs nothing. f.Admin() is a resource
// for the admin package's screens, to change them.
package flags

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/sqldb"
)

// Flag is a feature and who gets it.
type Flag struct {
	// Name identifies the flag in code, such as "new-editor".
	Name string `json:"name" admin:"key,list,search" validate:"required,max=100,pattern=[a-z0-9][a-z0-9._-]*" description:"Lowercase letters, digits, dots, dashes and underscores."`
	// Description says what it turns on, for whoever changes it.
	Description string `json:"description" admin:"list,search" validate:"max=500"`
	// On is the kill switch: off, nobody gets the feature.
	On bool `json:"on" admin:"list,filter" description:"Off, nobody gets it, whatever else is set."`
	// Percent of the subjects counted By get it, 0 to 100. Raising it keeps
	// those who had it: each subject's place is fixed by a hash.
	Percent int `json:"percent" admin:"list" validate:"min=0,max=100" description:"The share of users, teams or visitors who get it."`
	// By is what Percent counts: "user", "team" or "visitor", a browser
	// with a cookie.
	By string `json:"by" admin:"list" validate:"oneof=user|team|visitor" label:"Counted by"`
	// Users and Teams get it whatever the percentage, as IDs separated by
	// commas: the team that built it, the customers who asked for it.
	Users string `json:"users" admin:"textarea" description:"User IDs who always get it, separated by commas."`
	Teams string `json:"teams" admin:"textarea" description:"Team IDs who always get it, separated by commas."`
}

// Subject is who a request is for. Empty fields are unknown.
type Subject struct {
	User string
	Team string
}

// AccountsSubject is the signed-in user and their current team, from the
// accounts package. Add accounts' plugin before the flags'.
func AccountsSubject(c *kit.Ctx) Subject {
	var s Subject
	if u := accounts.Current(c); u != nil {
		s.User = fmt.Sprint(u.ID)
		if m := accounts.CurrentTeam(c); m != nil {
			s.Team = fmt.Sprint(m.Team.ID)
		}
	}
	return s
}

// Options configure flags.
type Options struct {
	// Subject says who a request is for. Without it, only visitors are
	// known, by their cookie.
	Subject func(c *kit.Ctx) Subject
	// Refresh is how often each process reads changes others made, such
	// as from the admin screens on another server. Default: 10 seconds.
	Refresh time.Duration
	// Cookie names the visitor cookie for flags counted by visitor.
	// Default: "tern_visitor".
	Cookie string
}

// Flags are an app's feature flags. Add them to the app with
// kit.WithPlugins, so they're read when it starts and kept fresh.
type Flags struct {
	db      *sql.DB
	d       sqldb.Dialect
	opts    Options
	defined []Flag

	mu    sync.RWMutex
	flags map[string]Flag

	stop chan struct{}
	once sync.Once
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,99}$`)

// New returns flags kept in db, SQLite or PostgreSQL, with the flags the
// code knows about. Those not in the database yet are added, off, when the
// app starts; flags already there keep how they were set.
func New(db *sql.DB, opts Options, defined ...Flag) (*Flags, error) {
	d, err := sqldb.Detect(db)
	if err != nil {
		return nil, err
	}
	if opts.Refresh <= 0 {
		opts.Refresh = 10 * time.Second
	}
	if opts.Cookie == "" {
		opts.Cookie = "tern_visitor"
	}
	for i, f := range defined {
		if !namePattern.MatchString(f.Name) {
			return nil, fmt.Errorf("flags: %q isn't a flag name: use lowercase letters, digits, dots, dashes and underscores", f.Name)
		}
		if f.By == "" {
			defined[i].By = "user"
		}
	}
	return &Flags{db: db, d: d, opts: opts, defined: defined, flags: map[string]Flag{}, stop: make(chan struct{})}, nil
}

const table = "tern_flags"

// Migrate creates the flags' table, and adds the flags the code defines
// that aren't in it. The plugin runs it when the app starts.
func (f *Flags) Migrate(ctx context.Context) error {
	if _, err := f.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+table+` (
		name TEXT PRIMARY KEY,
		description TEXT NOT NULL DEFAULT '',
		on_ BOOLEAN NOT NULL DEFAULT FALSE,
		percent INTEGER NOT NULL DEFAULT 0,
		by_ TEXT NOT NULL DEFAULT 'user',
		users TEXT NOT NULL DEFAULT '',
		teams TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return err
	}
	for _, fl := range f.defined {
		if _, err := f.db.ExecContext(ctx, f.d.Rebind(`INSERT INTO `+table+` (name, description, on_, percent, by_, users, teams) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (name) DO NOTHING`),
			fl.Name, fl.Description, fl.On, fl.Percent, fl.By, fl.Users, fl.Teams); err != nil {
			return err
		}
	}
	return f.Reload(ctx)
}

// Reload reads the flags from the database now.
func (f *Flags) Reload(ctx context.Context) error {
	rows, err := f.db.QueryContext(ctx, `SELECT name, description, on_, percent, by_, users, teams FROM `+table)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	got := map[string]Flag{}
	for rows.Next() {
		var fl Flag
		if err := rows.Scan(&fl.Name, &fl.Description, &fl.On, &fl.Percent, &fl.By, &fl.Users, &fl.Teams); err != nil {
			return err
		}
		got[fl.Name] = fl
	}
	if err := rows.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	f.flags = got
	f.mu.Unlock()
	return nil
}

// Set saves a flag, creating it if it's new, and applies it in this
// process at once; others pick it up within Options.Refresh.
func (f *Flags) Set(ctx context.Context, fl Flag) error {
	if !namePattern.MatchString(fl.Name) {
		return fmt.Errorf("flags: %q isn't a flag name", fl.Name)
	}
	if fl.By == "" {
		fl.By = "user"
	}
	if fl.By != "user" && fl.By != "team" && fl.By != "visitor" {
		return fmt.Errorf("flags: By is user, team or visitor, not %q", fl.By)
	}
	fl.Percent = min(max(fl.Percent, 0), 100)
	if _, err := f.db.ExecContext(ctx, f.d.Rebind(`INSERT INTO `+table+` (name, description, on_, percent, by_, users, teams) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, on_ = EXCLUDED.on_, percent = EXCLUDED.percent, by_ = EXCLUDED.by_, users = EXCLUDED.users, teams = EXCLUDED.teams`),
		fl.Name, fl.Description, fl.On, fl.Percent, fl.By, fl.Users, fl.Teams); err != nil {
		return err
	}
	f.mu.Lock()
	f.flags[fl.Name] = fl
	f.mu.Unlock()
	return nil
}

// Remove deletes a flag. Code that still checks it gets false.
func (f *Flags) Remove(ctx context.Context, name string) error {
	if _, err := f.db.ExecContext(ctx, f.d.Rebind(`DELETE FROM `+table+` WHERE name = ?`), name); err != nil {
		return err
	}
	f.mu.Lock()
	delete(f.flags, name)
	f.mu.Unlock()
	return nil
}

// List returns every flag, by name.
func (f *Flags) List() []Flag {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := slices.Collect(maps.Values(f.flags))
	slices.SortFunc(out, func(a, b Flag) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Get returns a flag, and whether there is one.
func (f *Flags) Get(name string) (Flag, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	fl, ok := f.flags[name]
	return fl, ok
}

// Decide says whether s gets the flag: never when it's off; always when s
// is in its users or teams; otherwise when s's place, a hash of the flag's
// name and s's ID counted By, is within Percent. visitor is the visitor's
// ID, for flags counted by visitor.
func (fl Flag) Decide(s Subject, visitor string) bool {
	if !fl.On {
		return false
	}
	if s.User != "" && listed(fl.Users, s.User) || s.Team != "" && listed(fl.Teams, s.Team) {
		return true
	}
	if fl.Percent >= 100 {
		return true
	}
	if fl.Percent <= 0 {
		return false
	}
	id := map[string]string{"user": s.User, "team": s.Team, "visitor": visitor}[fl.By]
	if id == "" {
		return false
	}
	return bucket(fl.Name, id) < fl.Percent*100
}

// bucket is a subject's place for a flag, 0 to 9999: the same for the
// same flag and ID, and unrelated between flags.
func bucket(name, id string) int {
	h := sha256.Sum256([]byte(name + "\x00" + id))
	return int(binary.BigEndian.Uint32(h[:4]) % 10000)
}

func listed(list, id string) bool {
	for v := range strings.SplitSeq(list, ",") {
		if strings.TrimSpace(v) == id {
			return true
		}
	}
	return false
}

// The request's subject, decisions and visitor, kept in its Locals.
const (
	subjectKey   = "tern.flags.subject"
	decisionsKey = "tern.flags" // kit's dev overlay shows it
)

// Enabled reports whether the request's subject gets the flag. A flag
// that doesn't exist is off. Decisions are the same for a subject on
// every request, until the flag changes.
func (f *Flags) Enabled(c *kit.Ctx, name string) bool {
	fl, ok := f.Get(name)
	if !ok {
		f.unknown(name)
		f.record(c, name, false)
		return false
	}
	s, _ := c.Locals[subjectKey].(*Subject)
	if s == nil {
		s = &Subject{}
		if f.opts.Subject != nil {
			*s = f.opts.Subject(c)
		}
		c.Locals[subjectKey] = s
	}
	visitor := ""
	if fl.On && fl.By == "visitor" && fl.Percent > 0 && fl.Percent < 100 {
		visitor = f.visitor(c)
	}
	on := fl.Decide(*s, visitor)
	f.record(c, name, on)
	return on
}

// For returns the decisions for these flags, such as for a layout's load
// to send the page: data.flags["new-editor"].
func (f *Flags) For(c *kit.Ctx, names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = f.Enabled(c, n)
	}
	return out
}

func (f *Flags) record(c *kit.Ctx, name string, on bool) {
	m, _ := c.Locals[decisionsKey].(map[string]bool)
	if m == nil {
		m = map[string]bool{}
		c.Locals[decisionsKey] = m
	}
	m[name] = on
}

var warned sync.Map

func (f *Flags) unknown(name string) {
	if _, seen := warned.LoadOrStore(name, true); !seen {
		slog.Warn("flags: checked a flag that doesn't exist; it's off", "flag", name)
	}
}

// visitor is the browser's ID from its cookie, set now if it has none.
func (f *Flags) visitor(c *kit.Ctx) string {
	if v, ok := c.Cookie(f.opts.Cookie); ok && len(v) == 32 {
		return v
	}
	if v, ok := c.Locals[f.opts.Cookie].(string); ok {
		return v
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	v := hex.EncodeToString(b[:])
	c.Locals[f.opts.Cookie] = v
	c.SetCookie(&http.Cookie{Name: f.opts.Cookie, Value: v, Path: "/", MaxAge: 400 * 24 * 3600, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: c.Request.TLS != nil})
	return v
}

// Name implements kit.Plugin.
func (f *Flags) Name() string { return "flags" }

// Setup implements kit.Plugin: the flags are read when the app starts,
// and again every Options.Refresh.
func (f *Flags) Setup(s *kit.Setup) error {
	s.OnStart(func(ctx context.Context) error {
		if err := f.Migrate(ctx); err != nil {
			return err
		}
		go f.refresh(s.Logger())
		return nil
	})
	s.OnShutdown(func(context.Context) error {
		f.once.Do(func() { close(f.stop) })
		return nil
	})
	return nil
}

func (f *Flags) refresh(log *slog.Logger) {
	t := time.NewTicker(f.opts.Refresh)
	defer t.Stop()
	for {
		select {
		case <-f.stop:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := f.Reload(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Warn("flags: reading the flags failed; keeping the ones read before", "err", err)
			}
			cancel()
		}
	}
}
