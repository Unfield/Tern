package search

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// sqlite keeps an index in an FTS5 table.
type sqlite struct {
	db    *sql.DB
	table string
	opts  Options
}

func (s *sqlite) migrate(ctx context.Context) error {
	cols := []string{"id UNINDEXED", "scope UNINDEXED"}
	for _, f := range s.opts.Fields {
		cols = append(cols, f.Name)
	}
	_, err := s.db.ExecContext(ctx, `CREATE VIRTUAL TABLE IF NOT EXISTS `+s.table+` USING fts5(`+strings.Join(cols, ", ")+`, tokenize = 'unicode61 remove_diacritics 2')`)
	return err
}

func (s *sqlite) put(ctx context.Context, d Doc) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.table+` WHERE id = ?`, d.ID); err != nil {
		return err
	}
	cols, marks := []string{"id", "scope"}, []string{"?", "?"}
	args := []any{d.ID, d.Scope}
	for _, f := range s.opts.Fields {
		cols = append(cols, f.Name)
		marks = append(marks, "?")
		args = append(args, d.Fields[f.Name])
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO `+s.table+` (`+strings.Join(cols, ", ")+`) VALUES (`+strings.Join(marks, ", ")+`)`, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqlite) remove(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM `+s.table+` WHERE id = ?`, id)
	return err
}

func (s *sqlite) reset(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM `+s.table)
	return err
}

// match is an FTS5 query: every word, as a quoted prefix, so operators in
// what was typed are only text.
func (s *sqlite) match(words []string) string {
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"*`
	}
	// Only the fields are searched, not the ID or the scope.
	names := make([]string, len(s.opts.Fields))
	for i, f := range s.opts.Fields {
		names[i] = f.Name
	}
	return "{" + strings.Join(names, " ") + "} : (" + strings.Join(parts, " ") + ")"
}

func (s *sqlite) search(ctx context.Context, words []string, q Query) (Results, error) {
	where := s.table + ` MATCH ?`
	args := []any{s.match(words)}
	if q.Scope != "" {
		where += ` AND scope = ?`
		args = append(args, q.Scope)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.table+` WHERE `+where, args...).Scan(&total); err != nil {
		return Results{}, err
	}
	// bm25 weighs each column, the ID and scope too; lower is better.
	weights := []string{"0", "0"}
	snippets := []string{}
	for i, f := range s.opts.Fields {
		weights = append(weights, fmt.Sprint(f.Boost))
		snippets = append(snippets, fmt.Sprintf(`snippet(%s, %d, char(1), char(2), '…', 16)`, s.table, i+2))
	}
	query := `SELECT id, scope, bm25(` + s.table + `, ` + strings.Join(weights, ", ") + `) AS rank, ` + strings.Join(snippets, ", ") +
		` FROM ` + s.table + ` WHERE ` + where + ` ORDER BY rank LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, query, append(args, q.Limit, q.Offset)...)
	if err != nil {
		return Results{}, err
	}
	defer func() { _ = rows.Close() }()
	res := Results{Hits: []Hit{}, Total: total}
	for rows.Next() {
		var h Hit
		var rank float64
		marked := make([]string, len(s.opts.Fields))
		dest := []any{&h.ID, &h.Scope, &rank}
		for i := range marked {
			dest = append(dest, &marked[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return Results{}, err
		}
		h.Score = -rank
		h.Snippets = map[string][]Segment{}
		for i, f := range s.opts.Fields {
			h.Snippets[f.Name] = segments(marked[i])
		}
		res.Hits = append(res.Hits, h)
	}
	return res, rows.Err()
}

// postgres keeps an index in a table with a tsvector column and a GIN index.
type postgres struct {
	db    *sql.DB
	table string
	opts  Options
}

// weights gives each field one of PostgreSQL's four weights, A for the
// highest boost.
func (p *postgres) weights() map[string]string {
	boosts := []float64{}
	seen := map[float64]bool{}
	for _, f := range p.opts.Fields {
		if !seen[f.Boost] {
			seen[f.Boost] = true
			boosts = append(boosts, f.Boost)
		}
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(boosts)))
	out := map[string]string{}
	for _, f := range p.opts.Fields {
		for i, b := range boosts {
			if b == f.Boost {
				out[f.Name] = string(rune('A' + min(i, 3)))
			}
		}
	}
	return out
}

func (p *postgres) migrate(ctx context.Context) error {
	cols := []string{"id TEXT PRIMARY KEY", "scope TEXT NOT NULL DEFAULT ''", "doc TSVECTOR NOT NULL"}
	for _, f := range p.opts.Fields {
		cols = append(cols, f.Name+" TEXT NOT NULL DEFAULT ''")
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS ` + p.table + ` (` + strings.Join(cols, ", ") + `)`,
		`CREATE INDEX IF NOT EXISTS ` + p.table + `_doc ON ` + p.table + ` USING GIN (doc)`,
		`CREATE INDEX IF NOT EXISTS ` + p.table + `_scope ON ` + p.table + ` (scope)`,
	} {
		if _, err := p.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (p *postgres) put(ctx context.Context, d Doc) error {
	w := p.weights()
	cols, vals, sets, docs := []string{"id", "scope"}, []string{"$1", "$2"}, []string{"scope = EXCLUDED.scope"}, []string{}
	args := []any{d.ID, d.Scope}
	for _, f := range p.opts.Fields {
		args = append(args, d.Fields[f.Name])
		n := fmt.Sprintf("$%d", len(args))
		cols = append(cols, f.Name)
		vals = append(vals, n)
		sets = append(sets, f.Name+" = EXCLUDED."+f.Name)
		// The words are indexed without accents, as queries are (the
		// unaccent extension isn't always installed); the field keeps them.
		args = append(args, foldText(d.Fields[f.Name]))
		docs = append(docs, fmt.Sprintf("setweight(to_tsvector('%s', $%d), '%s')", p.opts.Language, len(args), w[f.Name]))
	}
	cols = append(cols, "doc")
	vals = append(vals, strings.Join(docs, " || "))
	sets = append(sets, "doc = EXCLUDED.doc")
	_, err := p.db.ExecContext(ctx, `INSERT INTO `+p.table+` (`+strings.Join(cols, ", ")+`) VALUES (`+strings.Join(vals, ", ")+`) ON CONFLICT (id) DO UPDATE SET `+strings.Join(sets, ", "), args...)
	return err
}

func (p *postgres) remove(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM `+p.table+` WHERE id = $1`, id)
	return err
}

func (p *postgres) reset(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM `+p.table)
	return err
}

func (p *postgres) search(ctx context.Context, words []string, q Query) (Results, error) {
	// Each word as a prefix; Words keeps only letters and digits, so
	// nothing in them is tsquery syntax.
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = w + ":*"
	}
	tsq := fmt.Sprintf("to_tsquery('%s', $1)", p.opts.Language)
	where := `doc @@ ` + tsq
	args := []any{strings.Join(parts, " & ")}
	if q.Scope != "" {
		where += ` AND scope = $2`
		args = append(args, q.Scope)
	}
	var total int
	if err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+p.table+` WHERE `+where, args...).Scan(&total); err != nil {
		return Results{}, err
	}
	heads := []string{}
	for _, f := range p.opts.Fields {
		heads = append(heads, fmt.Sprintf(`ts_headline('%s', %s, %s, 'StartSel=%s, StopSel=%s, MaxWords=24, MinWords=10, MaxFragments=1, FragmentDelimiter=…')`, p.opts.Language, f.Name, tsq, "\x01", "\x02"))
	}
	n := len(args)
	names := make([]string, len(p.opts.Fields))
	for i, f := range p.opts.Fields {
		names[i] = f.Name
	}
	query := `SELECT id, scope, ts_rank(doc, ` + tsq + `) AS rank, ` + strings.Join(heads, ", ") + `, ` + strings.Join(names, ", ") +
		` FROM ` + p.table + ` WHERE ` + where + fmt.Sprintf(` ORDER BY rank DESC, id LIMIT $%d OFFSET $%d`, n+1, n+2)
	rows, err := p.db.QueryContext(ctx, query, append(args, q.Limit, q.Offset)...)
	if err != nil {
		return Results{}, err
	}
	defer func() { _ = rows.Close() }()
	res := Results{Hits: []Hit{}, Total: total}
	for rows.Next() {
		var h Hit
		marked := make([]string, len(p.opts.Fields))
		full := make([]string, len(p.opts.Fields))
		dest := []any{&h.ID, &h.Scope, &h.Score}
		for i := range marked {
			dest = append(dest, &marked[i])
		}
		for i := range full {
			dest = append(dest, &full[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return Results{}, err
		}
		h.Snippets = map[string][]Segment{}
		for i, f := range p.opts.Fields {
			// ts_headline cuts long fields without saying so: mark where.
			plain := strings.NewReplacer("\x01", "", "\x02", "").Replace(marked[i])
			if plain != "" && !strings.HasPrefix(full[i], plain) {
				marked[i] = "…" + marked[i]
			}
			if plain != "" && !strings.HasSuffix(full[i], plain) {
				marked[i] += "…"
			}
			h.Snippets[f.Name] = segments(marked[i])
		}
		res.Hits = append(res.Hits, h)
	}
	return res, rows.Err()
}
