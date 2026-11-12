// Package shelf keeps the links in a SQLite database.
package shelf

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/search"
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
	// Index finds links by their titles and addresses, each user's own.
	Index *search.Index
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
	idx, err := search.New(db, "links", search.Options{Fields: []search.Field{{Name: "title", Boost: 2}, {Name: "url"}}})
	if err != nil {
		return err
	}
	if err := idx.Migrate(ctx); err != nil {
		return err
	}
	DB, Accounts, Index = db, acc, idx
	return nil
}

// Reindex indexes every link again. The app does it when it starts.
func Reindex(ctx context.Context) error {
	if err := Index.Reset(ctx); err != nil {
		return err
	}
	rows, err := DB.QueryContext(ctx, `SELECT id, user_id, url, title FROM links`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var l Link
		var userID int64
		if err := rows.Scan(&l.ID, &userID, &l.URL, &l.Title); err != nil {
			return err
		}
		if err := index(ctx, userID, l); err != nil {
			return err
		}
	}
	return rows.Err()
}

// IndexLink indexes one link again, after the admin screens changed it.
func IndexLink(ctx context.Context, id int64) error {
	var l Link
	var userID int64
	err := DB.QueryRowContext(ctx, `SELECT id, user_id, url, title FROM links WHERE id = ?`, id).Scan(&l.ID, &userID, &l.URL, &l.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return Index.Delete(ctx, strconv.FormatInt(id, 10))
	}
	if err != nil {
		return err
	}
	return index(ctx, userID, l)
}

func index(ctx context.Context, userID int64, l Link) error {
	return Index.Put(ctx, search.Doc{ID: strconv.FormatInt(l.ID, 10), Scope: scope(userID), Fields: map[string]string{"title": l.Title, "url": l.URL}})
}

// scope is the search scope of a user's links.
func scope(userID int64) string { return "user:" + strconv.FormatInt(userID, 10) }

// Search finds a user's links whose title or address has the words of q.
func Search(ctx context.Context, userID int64, q string, limit int) (search.Results, error) {
	return Index.Search(ctx, search.Query{Text: q, Scope: scope(userID), Limit: limit})
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
	if err != nil {
		return Link{}, err
	}
	return l, index(ctx, userID, l)
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
	if err := changed(DB.ExecContext(ctx, `DELETE FROM links WHERE id = ? AND user_id = ?`, id, userID)); err != nil {
		return err
	}
	return Index.Delete(ctx, strconv.FormatInt(id, 10))
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

// IsAdmin reports whether u may use the admin screens: whether their ID
// is in SHELF_ADMINS, separated by commas. IDs, unlike email addresses,
// can't be taken by someone who signs up first: Shelf sends no emails, so
// it can't check that an address is the user's.
func IsAdmin(u *accounts.User) bool {
	if u == nil {
		return false
	}
	for id := range strings.SplitSeq(os.Getenv("SHELF_ADMINS"), ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64); err == nil && n == u.ID {
			return true
		}
	}
	return false
}

// Result is a link that matched a search, with the matches in its title
// and address marked.
type Result struct {
	ID      int64            `json:"id"`
	Href    string           `json:"href"`
	Title   []search.Segment `json:"title"`
	Snippet []search.Segment `json:"snippet"`
}

// Results is Search's hits as Results, and how many matched in all.
func Results(ctx context.Context, userID int64, q string, limit int) ([]Result, int, error) {
	res, err := Search(ctx, userID, q, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Result, 0, len(res.Hits))
	for _, h := range res.Hits {
		id, _ := strconv.ParseInt(h.ID, 10, 64)
		out = append(out, Result{ID: id, Href: "/links/" + h.ID, Title: h.Snippets["title"], Snippet: h.Snippets["url"]})
	}
	return out, res.Total, nil
}
