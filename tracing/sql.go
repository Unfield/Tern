package tracing

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"log/slog"
	"strings"
)

// OpenDB opens a database like sql.Open, with each query a client span
// inside the trace of the context it runs with:
//
//	db, err := tracing.OpenDB("sqlite", "app.db")
//
// Queries without a span in their context, such as at startup, record
// nothing.
func OpenDB(driverName, dsn string) (*sql.DB, error) {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	d := db.Driver()
	_ = db.Close()
	var c driver.Connector
	if dc, ok := d.(driver.DriverContext); ok {
		if c, err = dc.OpenConnector(dsn); err != nil {
			return nil, err
		}
	} else {
		c = dsnConnector{dsn: dsn, d: d}
	}
	return sql.OpenDB(WrapConnector(c, dbSystem(driverName))), nil
}

// dbSystem is OpenTelemetry's db.system.name for a driver.
func dbSystem(driverName string) string {
	switch driverName {
	case "pgx", "postgres", "pq":
		return "postgresql"
	case "sqlite", "sqlite3":
		return "sqlite"
	case "mysql":
		return "mysql"
	}
	return driverName
}

type dsnConnector struct {
	dsn string
	d   driver.Driver
}

func (c dsnConnector) Connect(context.Context) (driver.Conn, error) { return c.d.Open(c.dsn) }
func (c dsnConnector) Driver() driver.Driver                        { return c.d }

// WrapConnector makes each query through c a span, for sql.OpenDB.
// system is the database, as OpenTelemetry names it: "postgresql",
// "sqlite", "mysql".
func WrapConnector(c driver.Connector, system string) driver.Connector {
	return &connector{c: c, system: system}
}

type connector struct {
	c      driver.Connector
	system string
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.c.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &tracedConn{Conn: conn, system: c.system}, nil
}

func (c *connector) Driver() driver.Driver { return c.c.Driver() }

// tracedConn passes everything through, timing queries. Interfaces the driver's
// connection lacks answer driver.ErrSkip, so database/sql falls back as it
// would without the wrapper.
type tracedConn struct {
	driver.Conn
	system string
}

// span starts a query's span when the context is in a trace.
func (c *tracedConn) span(ctx context.Context, query string) (context.Context, *Span) {
	if FromContext(ctx) == nil {
		return ctx, nil
	}
	return Start(ctx, queryName(query), WithKind(Client), WithAttrs(
		slog.String("db.system.name", c.system),
		slog.String("db.query.text", truncate(query, 2000)),
	))
}

func end(span *Span, err error) {
	if err != nil && !errors.Is(err, driver.ErrSkip) && !errors.Is(err, sql.ErrNoRows) {
		span.RecordError(err)
	}
	span.End()
}

// queryName is the statement's first keyword: SELECT, INSERT, UPDATE…
func queryName(q string) string {
	q = strings.TrimSpace(q)
	for strings.HasPrefix(q, "--") || strings.HasPrefix(q, "/*") {
		if strings.HasPrefix(q, "--") {
			_, q, _ = strings.Cut(q, "\n")
		} else {
			_, q, _ = strings.Cut(q, "*/")
		}
		q = strings.TrimSpace(q)
	}
	word, _, _ := strings.Cut(q, " ")
	word = strings.ToUpper(strings.TrimRight(word, "(\n\t;"))
	if word == "" {
		return "SQL"
	}
	return word
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (c *tracedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	q, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	ctx, span := c.span(ctx, query)
	rows, err := q.QueryContext(ctx, query, args)
	end(span, err)
	return rows, err
}

func (c *tracedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	e, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	ctx, span := c.span(ctx, query)
	res, err := e.ExecContext(ctx, query, args)
	end(span, err)
	return res, err
}

func (c *tracedConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	var st driver.Stmt
	var err error
	if p, ok := c.Conn.(driver.ConnPrepareContext); ok {
		st, err = p.PrepareContext(ctx, query)
	} else {
		st, err = c.Conn.Prepare(query)
	}
	if err != nil {
		return nil, err
	}
	return &stmt{Stmt: st, c: c, query: query}, nil
}

func (c *tracedConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *tracedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if b, ok := c.Conn.(driver.ConnBeginTx); ok {
		return b.BeginTx(ctx, opts)
	}
	return c.Conn.Begin() //nolint:staticcheck // the driver has nothing newer
}

func (c *tracedConn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *tracedConn) ResetSession(ctx context.Context) error {
	if r, ok := c.Conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *tracedConn) IsValid() bool {
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

func (c *tracedConn) CheckNamedValue(v *driver.NamedValue) error {
	if n, ok := c.Conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(v)
	}
	return driver.ErrSkip
}

type stmt struct {
	driver.Stmt
	c     *tracedConn
	query string
}

func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	ctx, span := s.c.span(ctx, s.query)
	var res driver.Result
	var err error
	if e, ok := s.Stmt.(driver.StmtExecContext); ok {
		res, err = e.ExecContext(ctx, args)
	} else {
		var vals []driver.Value
		if vals, err = values(args); err == nil {
			res, err = s.Stmt.Exec(vals) //nolint:staticcheck // the driver has nothing newer
		}
	}
	end(span, err)
	return res, err
}

func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	ctx, span := s.c.span(ctx, s.query)
	var rows driver.Rows
	var err error
	if q, ok := s.Stmt.(driver.StmtQueryContext); ok {
		rows, err = q.QueryContext(ctx, args)
	} else {
		var vals []driver.Value
		if vals, err = values(args); err == nil {
			rows, err = s.Stmt.Query(vals) //nolint:staticcheck // the driver has nothing newer
		}
	}
	end(span, err)
	return rows, err
}

func (s *stmt) CheckNamedValue(v *driver.NamedValue) error {
	if n, ok := s.Stmt.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(v)
	}
	return s.c.CheckNamedValue(v)
}

func values(args []driver.NamedValue) ([]driver.Value, error) {
	out := make([]driver.Value, len(args))
	for i, a := range args {
		if a.Name != "" {
			return nil, errors.New("tracing: this driver doesn't take named arguments")
		}
		out[i] = a.Value
	}
	return out, nil
}
