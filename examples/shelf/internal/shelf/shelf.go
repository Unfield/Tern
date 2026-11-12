// Package shelf keeps the links in a SQLite database.
package shelf

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Unfield/Tern/accounts"
	_ "modernc.org/sqlite" // the SQLite driver, in pure Go
)

// Link is a page saved to read later.
type Link struct {
	ID      int64     `json:"id"`
	URL     string    `json:"url"`
	Title   string    `json:"title"`
	Read    bool      `json:"read"`
	AddedAt time.Time `json:"addedAt"`
}

// ErrNotFound is returned for a link that doesn't exist, or belongs to
// someone else.
var ErrNotFound = errors.New("link not found")

var (
	// DB is the database, opened by Open.
	DB *sql.DB
	// Accounts are the app's users, kept in the same database.
	Accounts *accounts.Accounts
)

// Open opens the SQLite database at path, creating it if needed, and sets
// up accounts in it.
func Open(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return err
	}
	acc, err := accounts.New(db, accounts.Options{})
	if err != nil {
		return err
	}
	if err := acc.Migrate(ctx); err != nil {
		return err
	}
	DB, Accounts = db, acc
	return nil
}

// List returns a user's links, the newest first. show is "unread", "read"
// or anything else for all of them.
func List(ctx context.Context, userID int64, show string) ([]Link, error) {
	query := `SELECT id, url, title, read, added_at FROM links WHERE user_id = ?`
	switch show {
	case "unread":
		query += ` AND NOT read`
	case "read":
		query += ` AND read`
	}
	rows, err := DB.QueryContext(ctx, query+` ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.ID, &l.URL, &l.Title, &l.Read, &l.AddedAt); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// Add saves a link for a user. Without a title, the address is the title.
func Add(ctx context.Context, userID int64, url, title string) (Link, error) {
	if title == "" {
		title = url
	}
	l := Link{URL: url, Title: title, AddedAt: time.Now().UTC()}
	err := DB.QueryRowContext(ctx, `INSERT INTO links (user_id, url, title, added_at) VALUES (?, ?, ?, ?) RETURNING id`,
		userID, l.URL, l.Title, l.AddedAt).Scan(&l.ID)
	return l, err
}

// Get returns one of a user's links.
func Get(ctx context.Context, userID, id int64) (Link, error) {
	var l Link
	err := DB.QueryRowContext(ctx, `SELECT id, url, title, read, added_at FROM links WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&l.ID, &l.URL, &l.Title, &l.Read, &l.AddedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	return l, err
}

// SetRead marks one of a user's links read or unread.
func SetRead(ctx context.Context, userID, id int64, read bool) error {
	return changed(DB.ExecContext(ctx, `UPDATE links SET read = ? WHERE id = ? AND user_id = ?`, read, id, userID))
}

// Delete removes one of a user's links.
func Delete(ctx context.Context, userID, id int64) error {
	return changed(DB.ExecContext(ctx, `DELETE FROM links WHERE id = ? AND user_id = ?`, id, userID))
}

// changed turns an update that matched no row into ErrNotFound.
func changed(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return ErrNotFound
	}
	return nil
}
