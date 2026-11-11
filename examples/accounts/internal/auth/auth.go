// Package auth sets up the accounts plugin: a SQLite database, and an
// outbox in place of an email provider.
package auth

import (
	"context"
	"database/sql"
	"log"
	"os"
	"sync"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"
	_ "modernc.org/sqlite"
)

// Acc is the app's accounts, for loads and actions.
var Acc *accounts.Accounts

// Open opens the database at $DATABASE (accounts.db by default) and
// creates the accounts tables.
func Open() (*accounts.Accounts, error) {
	path := os.Getenv("DATABASE")
	if path == "" {
		path = "accounts.db"
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// A real app sends email: accounts.SendWith(mailer, queue) with the
	// mail and jobs packages. This one keeps it in the outbox at /outbox.
	acc, err := accounts.New(db, accounts.Options{Send: send, RPName: "Tern accounts example"})
	if err != nil {
		return nil, err
	}
	if err := acc.Migrate(context.Background()); err != nil {
		return nil, err
	}
	Acc = acc
	return acc, nil
}

// Plugin installs acc.
func Plugin(acc *accounts.Accounts) kit.Option { return kit.WithPlugins(acc) }

var (
	mu     sync.Mutex
	outbox []accounts.Email
)

func send(_ context.Context, e accounts.Email) error {
	mu.Lock()
	defer mu.Unlock()
	outbox = append([]accounts.Email{e}, outbox...)
	if len(outbox) > 50 {
		outbox = outbox[:50]
	}
	log.Printf("email to %s: %s", e.To, e.Link)
	return nil
}

// Outbox is the emails sent, the newest first.
func Outbox() []accounts.Email {
	mu.Lock()
	defer mu.Unlock()
	return append([]accounts.Email(nil), outbox...)
}
