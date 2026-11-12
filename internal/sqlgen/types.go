package sqlgen

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the scratch database

	"github.com/Unfield/Tern/migrate"
)

// schemaColumn is a column of a table in the scratch database.
type schemaColumn struct {
	table, decl string
	notNull     bool
}

// scratch is the app's schema, built by its migrations in an in-memory
// SQLite database.
type scratch struct {
	db      *sql.DB
	columns map[string][]schemaColumn // by lowercased column name
	tables  map[string]map[string]schemaColumn
}

func openScratch(ctx context.Context, root string) (*scratch, error) {
	var id [8]byte
	_, _ = rand.Read(id[:])
	db, err := sql.Open("sqlite", "file:sqlgen-"+hex.EncodeToString(id[:])+"?mode=memory&cache=shared")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	// Queries run with made-up values, which needn't refer to real rows.
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if info, err := os.Stat(filepath.Join(root, "migrations")); err == nil && info.IsDir() {
		if _, err := migrate.Up(ctx, db, os.DirFS(root)); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("applying the migrations to type the queries: %w", err)
		}
	}
	s := &scratch{db: db, columns: map[string][]schemaColumn{}, tables: map[string]map[string]schemaColumn{}}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			_ = db.Close()
			return nil, err
		}
		tables = append(tables, name)
	}
	_ = rows.Close()
	for _, t := range tables {
		cols, err := db.QueryContext(ctx, `SELECT name, type, "notnull", pk FROM pragma_table_info(?)`, t)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		s.tables[strings.ToLower(t)] = map[string]schemaColumn{}
		for cols.Next() {
			var name, decl string
			var notNull, pk int
			if err := cols.Scan(&name, &decl, &notNull, &pk); err != nil {
				_ = cols.Close()
				_ = db.Close()
				return nil, err
			}
			// An INTEGER PRIMARY KEY is the rowid: never NULL.
			c := schemaColumn{table: t, decl: decl, notNull: notNull == 1 || pk > 0 && strings.EqualFold(decl, "INTEGER")}
			s.columns[strings.ToLower(name)] = append(s.columns[strings.ToLower(name)], c)
			s.tables[strings.ToLower(t)][strings.ToLower(name)] = c
		}
		_ = cols.Close()
	}
	return s, nil
}

func (s *scratch) close() { _ = s.db.Close() }

// goType maps a declared SQLite column type to a Go type, as the drivers
// scan it.
func goType(decl string) (string, bool) {
	d := strings.ToUpper(decl)
	if i := strings.IndexByte(d, '('); i >= 0 {
		d = strings.TrimSpace(d[:i])
	}
	switch {
	case d == "BOOLEAN" || d == "BOOL":
		return "bool", true
	case d == "DATE" || d == "DATETIME" || d == "TIMESTAMP" || d == "TIMESTAMPTZ" || d == "TIME":
		return "time.Time", true
	case d == "JSON" || d == "JSONB":
		return "json.RawMessage", true
	case strings.Contains(d, "INT") || strings.HasSuffix(d, "SERIAL"):
		return "int64", true
	case strings.Contains(d, "CHAR") || strings.Contains(d, "CLOB") || strings.Contains(d, "TEXT") || d == "UUID":
		return "string", true
	case strings.Contains(d, "BLOB") || d == "BYTEA":
		return "[]byte", true
	case strings.Contains(d, "REAL") || strings.Contains(d, "FLOA") || strings.Contains(d, "DOUBLE") || d == "NUMERIC" || strings.HasPrefix(d, "DECIMAL"):
		return "float64", true
	}
	return "", false
}

var (
	tableRe   = regexp.MustCompile(`(?i)\b(?:FROM|JOIN|INTO|UPDATE)\s+["` + "`" + `]?([A-Za-z_][A-Za-z0-9_]*)`)
	compareRe = regexp.MustCompile(`(?i)(?:([A-Za-z_][A-Za-z0-9_]*)\.)?([A-Za-z_][A-Za-z0-9_]*)\s*(?:=|!=|<>|<=|>=|<|>|\bLIKE\b|\bGLOB\b|\bIS\b)\s*:([A-Za-z_][A-Za-z0-9_]*)`)
	reverseRe = regexp.MustCompile(`(?i):([A-Za-z_][A-Za-z0-9_]*)\s*(?:=|!=|<>|<=|>=|<|>)\s*(?:([A-Za-z_][A-Za-z0-9_]*)\.)?([A-Za-z_][A-Za-z0-9_]*)`)
	literalRe = regexp.MustCompile(`(?i):([A-Za-z_][A-Za-z0-9_]*)\s*(?:=|!=|<>|\bLIKE\b)\s*('|[0-9])|('|[0-9])\s*(?:=|!=|<>)\s*:([A-Za-z_][A-Za-z0-9_]*)`)
	limitRe   = regexp.MustCompile(`(?i)\b(?:LIMIT|OFFSET)\s+:([A-Za-z_][A-Za-z0-9_]*)`)
	insertRe  = regexp.MustCompile(`(?is)\bINSERT\s+(?:OR\s+\w+\s+)?INTO\s+["` + "`" + `]?([A-Za-z_][A-Za-z0-9_]*)["` + "`" + `]?\s*\(([^)]*)\)\s*VALUES\s*\((.*?)\)\s*(?:ON\b|RETURNING\b|$)`)
	setRe     = regexp.MustCompile(`(?is)\bSET\b(.*?)(?:\bWHERE\b|\bRETURNING\b|$)`)
	assignRe  = regexp.MustCompile(`^\s*["` + "`" + `]?([A-Za-z_][A-Za-z0-9_]*)["` + "`" + `]?\s*=\s*:([A-Za-z_][A-Za-z0-9_]*)\s*$`)
	keywords  = map[string]bool{"select": true, "where": true, "and": true, "or": true, "not": true, "on": true, "set": true, "values": true, "null": true, "case": true, "when": true, "then": true, "else": true}
	returnRe  = regexp.MustCompile(`(?i)\bRETURNING\b`)
)

// inferParams gives each parameter without a "-- param" line the type of
// the column it's compared with, inserted into or set to.
func (s *scratch) inferParams(q *query) []string {
	var tables []string
	for _, m := range tableRe.FindAllStringSubmatch(q.raw, -1) {
		tables = append(tables, strings.ToLower(m[1]))
	}
	found := map[string][]string{} // param -> candidate types
	add := func(p, t string) { found[p] = append(found[p], t) }
	lookup := func(table, col string) (schemaColumn, bool) {
		col = strings.ToLower(col)
		if table != "" {
			if c, ok := s.tables[strings.ToLower(table)][col]; ok {
				return c, true
			}
		}
		var match []schemaColumn
		for _, c := range s.columns[col] {
			for _, t := range tables {
				if strings.EqualFold(c.table, t) {
					match = append(match, c)
				}
			}
		}
		if len(match) == 0 {
			return schemaColumn{}, false
		}
		for _, c := range match[1:] {
			if c.decl != match[0].decl {
				return schemaColumn{}, false
			}
		}
		return match[0], true
	}
	typeOf := func(c schemaColumn, writes bool) string {
		t, ok := goType(c.decl)
		if !ok {
			return ""
		}
		if writes && !c.notNull {
			return "*" + t
		}
		return t
	}
	if m := insertRe.FindStringSubmatch(q.raw); m != nil {
		cols := strings.Split(m[2], ",")
		vals := splitTop(m[3])
		if len(cols) == len(vals) {
			for i, v := range vals {
				v = strings.TrimSpace(v)
				if strings.HasPrefix(v, ":") && isIdentRun(v[1:]) {
					if c, ok := s.tables[strings.ToLower(m[1])][strings.ToLower(strings.Trim(strings.TrimSpace(cols[i]), "\"`"))]; ok {
						add(v[1:], typeOf(c, true))
					}
				}
			}
		}
	}
	if m := setRe.FindStringSubmatch(q.raw); m != nil && len(tables) > 0 {
		for _, part := range splitTop(m[1]) {
			if a := assignRe.FindStringSubmatch(part); a != nil {
				if c, ok := s.tables[tables[0]][strings.ToLower(a[1])]; ok {
					add(a[2], typeOf(c, true))
				}
			}
		}
	}
	for _, m := range compareRe.FindAllStringSubmatch(q.raw, -1) {
		if keywords[strings.ToLower(m[2])] {
			continue
		}
		if c, ok := lookup(m[1], m[2]); ok {
			add(m[3], typeOf(c, false))
		}
	}
	for _, m := range reverseRe.FindAllStringSubmatch(q.raw, -1) {
		if keywords[strings.ToLower(m[3])] {
			continue
		}
		if c, ok := lookup(m[2], m[3]); ok {
			add(m[1], typeOf(c, false))
		}
	}
	for _, m := range limitRe.FindAllStringSubmatch(q.raw, -1) {
		add(m[1], "int64")
	}
	// Compared with a literal: a string, or a number.
	for _, m := range literalRe.FindAllStringSubmatch(q.raw, -1) {
		name, lit := m[1], m[2]
		if name == "" {
			name, lit = m[4], m[3]
		}
		if lit == "'" {
			add(name, "string")
		} else {
			add(name, "int64")
		}
	}

	var errs []string
	for _, p := range q.params {
		if t, ok := q.paramTypes[p.name]; ok {
			p.goType = t
			continue
		}
		var types []string
		for _, t := range found[p.name] {
			if t != "" && !contains(types, t) {
				types = append(types, t)
			}
		}
		// A parameter both compared with and written to a nullable column
		// is written: a pointer covers both.
		if len(types) == 2 && (types[0] == "*"+types[1] || types[1] == "*"+types[0]) {
			types = []string{"*" + strings.TrimPrefix(types[0], "*")}
		}
		switch len(types) {
		case 1:
			p.goType = types[0]
		case 0:
			errs = append(errs, fmt.Sprintf("%s:%d: %s: can't tell the type of :%s from the schema; say it: -- param %s int64 (or string, bool, float64, time.Time, []byte)", q.file, q.line, q.name, p.name, p.name))
		default:
			errs = append(errs, fmt.Sprintf("%s:%d: %s: :%s is used as %s; say which: -- param %s %s", q.file, q.line, q.name, p.name, strings.Join(types, " and "), p.name, types[0]))
		}
	}
	return errs
}

// splitTop splits s at commas outside parentheses and quotes.
func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func isIdentRun(s string) bool {
	if s == "" || !isIdentStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isIdent(s[i]) {
			return false
		}
	}
	return true
}

func contains(list []string, s string) bool { return slices.Contains(list, s) }

// sample is a value of a Go type, to run a query with.
func sample(t string) any {
	if strings.HasPrefix(t, "*") {
		return nil
	}
	switch t {
	case "int64", "int":
		return int64(1)
	case "float64":
		return 1.0
	case "bool":
		return true
	case "time.Time":
		return time.Unix(0, 0).UTC()
	case "[]byte":
		return []byte{}
	case "json.RawMessage":
		return "{}"
	}
	return "x"
}

// typeColumns runs q in a transaction it rolls back, to check it against
// the schema and read the types of what it returns.
func (s *scratch) typeColumns(ctx context.Context, q *query) error {
	at := fmt.Sprintf("%s:%d: %s", q.file, q.line, q.name)
	returns := q.kind == kindOne || q.kind == kindMany
	if !returns && returnRe.MatchString(q.sql) {
		return fmt.Errorf("%s: the query returns rows (RETURNING); make it :one or :many", at)
	}
	if q.unchecked {
		for _, p := range q.params {
			if p.goType == "" {
				return fmt.Errorf("%s: an unchecked query needs a -- param line for :%s", at, p.name)
			}
		}
		if returns && len(q.columnOrder) == 0 {
			return fmt.Errorf("%s: an unchecked query needs a -- column line for each column it returns, in order", at)
		}
		for _, name := range q.columnOrder {
			q.columns = append(q.columns, column{name: name, goType: q.columnTypes[name]})
		}
		return nil
	}
	args := make([]any, len(q.uses))
	for i, u := range q.uses {
		args[i] = sample(q.params[u].goType)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if !returns {
		if _, err := tx.ExecContext(ctx, q.sql, args...); err != nil {
			return fmt.Errorf("%s: %v", at, sqliteError(err))
		}
		return nil
	}
	rows, err := tx.QueryContext(ctx, q.sql, args...)
	if err != nil {
		return fmt.Errorf("%s: %v", at, sqliteError(err))
	}
	defer func() { _ = rows.Close() }()
	names, err := rows.Columns()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("%s: the query returns no columns; make it :exec", at)
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return err
	}
	// Expressions have no declared type; a row may tell.
	var stepped []*sql.ColumnType
	if rows.Next() {
		stepped, _ = rows.ColumnTypes()
	}
	var tables []string
	for _, m := range tableRe.FindAllStringSubmatch(q.raw, -1) {
		tables = append(tables, strings.ToLower(m[1]))
	}
	seen := map[string]bool{}
	for i, name := range names {
		if seen[strings.ToLower(name)] {
			return fmt.Errorf("%s: two columns are named %s; name one with AS", at, name)
		}
		seen[strings.ToLower(name)] = true
		if !isIdentRun(name) {
			return fmt.Errorf("%s: column %d is named %q; name it with AS, such as AS total", at, i+1, name)
		}
		if t, ok := q.columnTypes[name]; ok {
			q.columns = append(q.columns, column{name: name, goType: t})
			continue
		}
		decl := types[i].DatabaseTypeName()
		t, ok := goType(decl)
		if st := scanType(stepped, i); !ok && st != "" {
			switch st {
			case "int64":
				t, ok = "int64", true
			case "float64":
				t, ok = "float64", true
			case "string":
				t, ok = "string", true
			case "[]uint8":
				t, ok = "[]byte", true
			}
		}
		if !ok {
			return fmt.Errorf("%s: can't tell the type of column %s; say it: -- column %s int64 (or string, bool, float64, time.Time, []byte; *int64 if it can be NULL)", at, name, name)
		}
		// Nullable when it's a column of a table that allows NULL.
		nullable := false
		for _, c := range s.columns[strings.ToLower(name)] {
			if strings.EqualFold(c.decl, decl) && contains(tables, strings.ToLower(c.table)) && !c.notNull {
				nullable = true
			}
		}
		if nullable {
			t = "*" + t
		}
		q.columns = append(q.columns, column{name: name, goType: t})
	}
	return rows.Err()
}

// sqliteError trims the driver's prefix from an error.
func sqliteError(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i > 0 && strings.Contains(msg[:i], "SQL logic error") {
		msg = msg[i+2:]
	}
	return strings.TrimSuffix(strings.TrimPrefix(msg, "SQL logic error: "), " (1)")
}

// scanType is the Go type the driver scans a stepped row's column into, or
// "" when there's no row or the value is NULL.
func scanType(cols []*sql.ColumnType, i int) string {
	if i >= len(cols) || cols[i].ScanType() == nil {
		return ""
	}
	return cols[i].ScanType().String()
}
