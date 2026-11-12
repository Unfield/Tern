package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/Unfield/Tern/internal/formrules"
	"github.com/Unfield/Tern/sqldb"
)

// ErrNotFound is what a Store returns for a record that doesn't exist.
var ErrNotFound = errors.New("admin: not found")

// Query is what a list asks a Store for.
type Query struct {
	// Search is text to look for in the searchable fields, or "".
	Search string
	// Filters maps columns to the values they must equal, as forms post them.
	Filters map[string]string
	// Sort is the column to sort by, or "" for the store's own order.
	Sort string
	Desc bool
	// Offset and Limit page the results.
	Offset, Limit int
}

// Store keeps the records of one kind. Table is a Store for a SQL table;
// implement it for data kept elsewhere, such as behind an API.
type Store[T any] interface {
	// List returns a page of the records that match, and how many match in all.
	List(ctx context.Context, q Query) (records []T, total int, err error)
	// Get returns the record with this key, or ErrNotFound.
	Get(ctx context.Context, key string) (T, error)
	// Create adds a record and returns its key.
	Create(ctx context.Context, record T) (key string, err error)
	// Update replaces the record with this key. columns are the ones the
	// form edited.
	Update(ctx context.Context, key string, record T, columns []string) error
	// Delete removes the record with this key.
	Delete(ctx context.Context, key string) error
}

// Table is a Store for a SQL table, in SQLite or PostgreSQL, whose
// columns are T's fields: each field's db tag, else its name in snake_case.
type Table[T any] struct {
	db      *sql.DB
	dialect sqldb.Dialect
	table   string
	fields  []field
	order   string
}

// NewTable returns a Store for table. order is the list's order when
// nobody chose one, such as "id DESC"; "" is the key, newest first.
func NewTable[T any](db *sql.DB, table, order string) (*Table[T], error) {
	d, err := sqldb.Detect(db)
	if err != nil {
		return nil, err
	}
	fields, err := fieldsOf(reflect.TypeFor[T]())
	if err != nil {
		return nil, err
	}
	t := &Table[T]{db: db, dialect: d, table: table, fields: fields, order: order}
	if t.order == "" {
		t.order = quote(t.key().column) + " DESC"
	}
	return t, nil
}

func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func (t *Table[T]) key() *field {
	for i := range t.fields {
		if t.fields[i].key {
			return &t.fields[i]
		}
	}
	return nil
}

func (t *Table[T]) columns() string {
	cols := make([]string, len(t.fields))
	for i, f := range t.fields {
		cols[i] = quote(f.column)
	}
	return strings.Join(cols, ", ")
}

func (t *Table[T]) field(column string) *field {
	for i := range t.fields {
		if t.fields[i].column == column {
			return &t.fields[i]
		}
	}
	return nil
}

// arg turns a posted value into a value for the database.
func (t *Table[T]) arg(f *field, s string) (any, error) {
	v := reflect.New(f.typ).Elem()
	if err := f.set(v, s); err != nil {
		return nil, err
	}
	return v.Interface(), nil
}

func (t *Table[T]) where(q Query) (string, []any, error) {
	var conds []string
	var args []any
	if q.Search != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(q.Search)) + "%"
		var or []string
		for _, f := range t.fields {
			if f.search {
				or = append(or, "LOWER("+quote(f.column)+`) LIKE ? ESCAPE '\'`)
				args = append(args, like)
			}
		}
		if len(or) > 0 {
			conds = append(conds, "("+strings.Join(or, " OR ")+")")
		}
	}
	for col, s := range q.Filters {
		f := t.field(col)
		if f == nil {
			return "", nil, fmt.Errorf("admin: no column %q to filter by", col)
		}
		v, err := t.arg(f, s)
		if err != nil {
			return "", nil, err
		}
		conds = append(conds, quote(col)+" = ?")
		args = append(args, v)
	}
	if len(conds) == 0 {
		return "", nil, nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args, nil
}

func (t *Table[T]) scan(rows interface{ Scan(...any) error }) (T, error) {
	var rec T
	v := reflect.ValueOf(&rec).Elem()
	dest := make([]any, len(t.fields))
	for i, f := range t.fields {
		dest[i] = v.FieldByIndex(f.index).Addr().Interface()
	}
	err := rows.Scan(dest...)
	return rec, err
}

// List implements Store.
func (t *Table[T]) List(ctx context.Context, q Query) ([]T, int, error) {
	where, args, err := t.where(q)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := t.db.QueryRowContext(ctx, t.dialect.Rebind("SELECT COUNT(*) FROM "+quote(t.table)+where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := t.order
	if q.Sort != "" {
		if t.field(q.Sort) == nil {
			return nil, 0, fmt.Errorf("admin: no column %q to sort by", q.Sort)
		}
		order = quote(q.Sort)
		if q.Desc {
			order += " DESC"
		}
		// The key keeps pages stable when many rows sort the same.
		order += ", " + quote(t.key().column)
	}
	query := "SELECT " + t.columns() + " FROM " + quote(t.table) + where + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	rows, err := t.db.QueryContext(ctx, t.dialect.Rebind(query), append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := []T{}
	for rows.Next() {
		rec, err := t.scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, rec)
	}
	return out, total, rows.Err()
}

// Get implements Store.
func (t *Table[T]) Get(ctx context.Context, key string) (T, error) {
	k, err := t.arg(t.key(), key)
	if err != nil {
		var zero T
		return zero, ErrNotFound
	}
	row := t.db.QueryRowContext(ctx, t.dialect.Rebind("SELECT "+t.columns()+" FROM "+quote(t.table)+" WHERE "+quote(t.key().column)+" = ?"), k)
	rec, err := t.scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return rec, ErrNotFound
	}
	return rec, err
}

// Create implements Store. Read-only fields are left to the database's
// defaults, and so is a numeric key that is zero.
func (t *Table[T]) Create(ctx context.Context, rec T) (string, error) {
	v := reflect.ValueOf(rec)
	key := t.key()
	var cols, marks []string
	var args []any
	for _, f := range t.fields {
		fv := v.FieldByIndex(f.index)
		if f.readonly && !f.key || f.key && f.rules.Kind != formrules.String && fv.IsZero() {
			continue
		}
		cols = append(cols, quote(f.column))
		marks = append(marks, "?")
		args = append(args, fv.Interface())
	}
	query := "INSERT INTO " + quote(t.table) + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(marks, ", ") + ") RETURNING " + quote(key.column)
	k := reflect.New(key.typ)
	if err := t.db.QueryRowContext(ctx, t.dialect.Rebind(query), args...).Scan(k.Interface()); err != nil {
		return "", err
	}
	return key.text(k.Elem()), nil
}

// Update implements Store.
func (t *Table[T]) Update(ctx context.Context, key string, rec T, columns []string) error {
	k, err := t.arg(t.key(), key)
	if err != nil {
		return ErrNotFound
	}
	v := reflect.ValueOf(rec)
	var sets []string
	var args []any
	for _, col := range columns {
		f := t.field(col)
		if f == nil || f.key || f.readonly {
			continue
		}
		sets = append(sets, quote(col)+" = ?")
		args = append(args, v.FieldByIndex(f.index).Interface())
	}
	if len(sets) == 0 {
		return nil
	}
	res, err := t.db.ExecContext(ctx, t.dialect.Rebind("UPDATE "+quote(t.table)+" SET "+strings.Join(sets, ", ")+" WHERE "+quote(t.key().column)+" = ?"), append(args, k)...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete implements Store.
func (t *Table[T]) Delete(ctx context.Context, key string) error {
	k, err := t.arg(t.key(), key)
	if err != nil {
		return ErrNotFound
	}
	res, err := t.db.ExecContext(ctx, t.dialect.Rebind("DELETE FROM "+quote(t.table)+" WHERE "+quote(t.key().column)+" = ?"), k)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}
