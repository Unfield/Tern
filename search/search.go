// Package search is full-text search for an app's data and content: in
// SQLite with FTS5, in PostgreSQL with tsvector and a GIN index, or in
// memory, for content collections and other data that fits in it.
//
// An index has fields, such as a title and a body, and documents with an
// ID, a scope and a value for each field:
//
//	idx, err := search.New(db, "links", search.Options{
//		Fields: []search.Field{{Name: "title", Boost: 2}, {Name: "url"}},
//	})
//	err = idx.Migrate(ctx)
//	err = idx.Put(ctx, search.Doc{ID: "42", Scope: "user:7", Fields: map[string]string{"title": "Go 1.26", "url": "https://go.dev"}})
//	res, err := idx.Search(ctx, search.Query{Text: "go", Scope: "user:7"})
//
// Searches match words as typed, as prefixes of words in the documents, so
// results come in while someone types, and rank them with BM25 (SQLite and
// memory) or ts_rank (PostgreSQL). Each hit has highlighted snippets of
// its fields, as segments of text rather than HTML. Tern UI's SearchBox
// shows them.
package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/Unfield/Tern/internal/htmltext"
	"github.com/Unfield/Tern/sqldb"
)

// Field is a field of an index's documents.
type Field struct {
	// Name is the field's name, a letter followed by letters, digits and
	// underscores.
	Name string
	// Boost weighs matches in the field: 2 counts them twice. Default 1.
	Boost float64
}

// Options configure an index.
type Options struct {
	// Fields are the documents' fields, the most important first.
	Fields []Field
	// Language is PostgreSQL's text search configuration, for word stems
	// such as "running" matching "run": "english" by default, "simple"
	// for none. SQLite and memory indexes match words without stems.
	Language string
}

// Doc is a document in an index.
type Doc struct {
	// ID identifies the document, such as its row's key or a page's slug.
	ID string
	// Scope is who the document belongs to, such as "user:7" or "team:3":
	// a query with a scope finds only its documents. "" is no one's.
	Scope string
	// Fields holds the text of each field. Fields not in the index are ignored.
	Fields map[string]string
}

// Query is a search.
type Query struct {
	// Text is what was typed. Each word must match, as a word or the
	// start of one; quotes and operators are taken as text.
	Text string
	// Scope limits the search to documents with this scope; "" searches
	// documents of every scope.
	Scope string
	// Limit is how many hits to return, 20 by default; Offset skips some, for pages.
	Limit, Offset int
}

// Segment is a piece of a snippet: text that matched the query, or text around it.
type Segment struct {
	Text  string `json:"text"`
	Match bool   `json:"match,omitempty"`
}

// Hit is a document that matched.
type Hit struct {
	ID    string  `json:"id"`
	Scope string  `json:"scope,omitempty"`
	Score float64 `json:"score"`
	// Snippets are the fields' text around what matched, with the matches
	// marked; long fields are cut to a few words around the first match.
	Snippets map[string][]Segment `json:"snippets"`
}

// Results are a page of hits.
type Results struct {
	Hits []Hit `json:"hits"`
	// Total is how many documents match in all.
	Total int `json:"total"`
}

// backend is where an index keeps its documents.
type backend interface {
	migrate(ctx context.Context) error
	put(ctx context.Context, d Doc) error
	remove(ctx context.Context, id string) error
	search(ctx context.Context, words []string, q Query) (Results, error)
	reset(ctx context.Context) error
}

// Index is a search index. It's safe for concurrent use.
type Index struct {
	opts Options
	b    backend
}

var fieldName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

func check(name string, opts *Options) error {
	if name != "" && !fieldName.MatchString(name) {
		return fmt.Errorf("search: index name %q must be a letter followed by letters, digits and underscores", name)
	}
	if len(opts.Fields) == 0 {
		return errors.New("search: an index needs at least one field")
	}
	seen := map[string]bool{"id": true, "scope": true, "doc": true}
	for i, f := range opts.Fields {
		if !fieldName.MatchString(f.Name) || seen[strings.ToLower(f.Name)] {
			return fmt.Errorf("search: field name %q is invalid or taken", f.Name)
		}
		seen[strings.ToLower(f.Name)] = true
		if f.Boost <= 0 {
			opts.Fields[i].Boost = 1
		}
	}
	if opts.Language == "" {
		opts.Language = "english"
	}
	if !fieldName.MatchString(opts.Language) {
		return fmt.Errorf("search: language %q is invalid", opts.Language)
	}
	return nil
}

// New returns an index named name in db, SQLite or PostgreSQL. Call
// Migrate to create its table.
func New(db *sql.DB, name string, opts Options) (*Index, error) {
	opts.Fields = append([]Field(nil), opts.Fields...)
	if err := check(name, &opts); err != nil {
		return nil, err
	}
	d, err := sqldb.Detect(db)
	if err != nil {
		return nil, err
	}
	table := "tern_search_" + name
	if d == sqldb.Postgres {
		return &Index{opts: opts, b: &postgres{db: db, table: table, opts: opts}}, nil
	}
	return &Index{opts: opts, b: &sqlite{db: db, table: table, opts: opts}}, nil
}

// NewMemory returns an index in memory, for content collections and
// other documents that fit in memory and are indexed when the app starts.
func NewMemory(opts Options) (*Index, error) {
	opts.Fields = append([]Field(nil), opts.Fields...)
	if err := check("", &opts); err != nil {
		return nil, err
	}
	return &Index{opts: opts, b: newMemory(opts)}, nil
}

// Migrate creates the index's table, if it doesn't exist yet.
func (x *Index) Migrate(ctx context.Context) error { return x.b.migrate(ctx) }

// Put adds a document, or replaces the one with its ID.
func (x *Index) Put(ctx context.Context, d Doc) error {
	if d.ID == "" {
		return errors.New("search: a document needs an ID")
	}
	return x.b.put(ctx, d)
}

// Delete removes the document with this ID; there may be none.
func (x *Index) Delete(ctx context.Context, id string) error { return x.b.remove(ctx, id) }

// Reset removes every document, to index everything again.
func (x *Index) Reset(ctx context.Context) error { return x.b.reset(ctx) }

// Search returns the documents that match q, the best first.
func (x *Index) Search(ctx context.Context, q Query) (Results, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	q.Limit = min(q.Limit, 200)
	q.Offset = max(q.Offset, 0)
	words := Words(q.Text)
	if len(words) == 0 {
		return Results{Hits: []Hit{}}, nil
	}
	return x.b.search(ctx, words, q)
}

// Words splits text into lowercase words, as queries and memory indexes
// see them: letters and digits, with accents removed.
func Words(text string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur = append(cur, fold(r))
			continue
		}
		if unicode.Is(unicode.Mn, r) {
			continue // a combining accent
		}
		flush()
	}
	flush()
	if len(out) > 16 {
		out = out[:16]
	}
	return out
}

// fold lowercases r and drops its accent, for the common Latin letters.
func fold(r rune) rune {
	r = unicode.ToLower(r)
	if r < 0xC0 || r > 0x17F {
		return r
	}
	const from = "àáâãäåāăąçćĉċčďđèéêëēĕėęěĝğġģĥħìíîïĩīĭįıĵķĺļľŀłñńņňòóôõöøōŏőŕŗřśŝşšţťŧùúûüũūŭůűųŵýÿŷźżž"
	const to = "aaaaaaaaacccccddeeeeeeeeegggghhiiiiiiiiijklllllnnnnoooooooooorrrsssstttuuuuuuuuuuwyyyzzz"
	if i := strings.IndexRune(from, r); i >= 0 {
		return []rune(to)[len([]rune(from[:i]))]
	}
	return r
}

// foldText lowercases text and drops the accents of its letters.
func foldText(text string) string {
	var b strings.Builder
	for _, r := range text {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(fold(r))
	}
	return b.String()
}

// segments splits text marked with \x01 before and \x02 after each match.
func segments(marked string) []Segment {
	out := []Segment{}
	match := false
	for marked != "" {
		sep := "\x01"
		if match {
			sep = "\x02"
		}
		part, rest, found := strings.Cut(marked, sep)
		if part != "" {
			out = append(out, Segment{Text: part, Match: match})
		}
		if !found {
			break
		}
		marked, match = rest, !match
	}
	return out
}

// Text is the text of an HTML document or fragment, such as a content
// entry's HTML, without its tags, for indexing.
func Text(html string) string { return htmltext.Convert(html) }
