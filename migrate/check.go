package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/Unfield/Tern/sqldb"
)

// Level is how serious a Problem is.
type Level string

// Levels of problems.
const (
	// Error is a migration that fails, or will on a database with data.
	Error Level = "error"
	// Warning is a change that breaks the version of the app still
	// running while a deploy rolls out, or loses data; a "-- safe:"
	// comment on the statement acknowledges it.
	Warning Level = "warning"
	// Note is worth knowing and needs nothing done.
	Note Level = "note"
)

// Problem is something Check found in a migration.
type Problem struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Level   Level  `json:"level"`
	Message string `json:"message"`
}

func (p Problem) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", p.File, p.Line, p.Level, p.Message)
}

// CheckOptions configure Check.
type CheckOptions struct {
	// Dialect is the database the app runs on; some changes only lock
	// tables on PostgreSQL.
	Dialect sqldb.Dialect
	// DB, when set, is an empty SQLite database to run the migrations on:
	// up, down and up again, which finds migrations that fail and down
	// sections that don't undo them.
	DB *sql.DB
}

// statement is one SQL statement of a migration, with the line it starts
// on and whether a "-- safe:" comment acknowledges it.
type statement struct {
	sql  string
	line int
	safe bool
}

var (
	dropTableRe   = regexp.MustCompile(`(?is)^DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?["` + "`" + `]?(\w+)`)
	dropColumnRe  = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+["` + "`" + `]?(\w+)["` + "`" + `]?\s+DROP\s+(?:COLUMN\s+)?(?:IF\s+EXISTS\s+)?["` + "`" + `]?(\w+)`)
	renameRe      = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+["` + "`" + `]?(\w+)["` + "`" + `]?\s+RENAME\s+(?:COLUMN\s+)?(?:["` + "`" + `]?(\w+)["` + "`" + `]?\s+)?TO\s+["` + "`" + `]?(\w+)`)
	addColumnRe   = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+["` + "`" + `]?(\w+)["` + "`" + `]?\s+ADD\s+(?:COLUMN\s+)?(?:IF\s+NOT\s+EXISTS\s+)?["` + "`" + `]?(\w+)["` + "`" + `]?(.*)$`)
	createTableRe = regexp.MustCompile(`(?is)^CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?["` + "`" + `]?(\w+)`)
	alterTypeRe   = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+["` + "`" + `]?(\w+)["` + "`" + `]?\s+ALTER\s+(?:COLUMN\s+)?["` + "`" + `]?(\w+)["` + "`" + `]?\s+(?:SET\s+DATA\s+)?TYPE\b`)
	updateRe      = regexp.MustCompile(`(?is)^UPDATE\s+["` + "`" + `]?(\w+)`)
	deleteRe      = regexp.MustCompile(`(?is)^DELETE\s+FROM\s+["` + "`" + `]?(\w+)`)
	truncateRe    = regexp.MustCompile(`(?is)^TRUNCATE\b`)
	whereRe       = regexp.MustCompile(`(?i)\bWHERE\b`)
	notNullRe     = regexp.MustCompile(`(?i)\bNOT\s+NULL\b`)
	defaultRe     = regexp.MustCompile(`(?i)\bDEFAULT\b|\bGENERATED\b`)
)

// Check looks for migrations in fsys that fail, or break the running app
// while a deploy rolls out: see Level for what each finding means.
func Check(ctx context.Context, fsys fs.FS, opts CheckOptions) ([]Problem, error) {
	ms, err := Load(fsys)
	if err != nil {
		return nil, err
	}
	var out []Problem
	created := map[string]string{} // table -> the file that made it
	for _, m := range ms {
		add := func(line int, level Level, format string, args ...any) {
			out = append(out, Problem{File: m.file, Line: line, Level: level, Message: fmt.Sprintf(format, args...)})
		}
		// The up section starts after any "-- +up" line; count from the
		// file's first line.
		for _, st := range splitStatements(m.up) {
			s := st.sql
			newTable := func(t string) bool { return created[strings.ToLower(t)] == m.file }
			switch {
			case createTableRe.MatchString(s):
				created[strings.ToLower(createTableRe.FindStringSubmatch(s)[1])] = m.file
			case dropTableRe.MatchString(s):
				t := dropTableRe.FindStringSubmatch(s)[1]
				if !st.safe && !newTable(t) {
					add(st.line, Warning, "drops the table %s: the version of the app running during the deploy may still use it. Stop using it in one release and drop it in the next, then mark this statement -- safe: <why>", t)
				}
			case dropColumnRe.MatchString(s):
				m := dropColumnRe.FindStringSubmatch(s)
				if !st.safe && !newTable(m[1]) {
					add(st.line, Warning, "drops the column %s.%s: the version of the app running during the deploy may still read it. Stop using it in one release and drop it in the next, then mark this statement -- safe: <why>", m[1], m[2])
				}
			case renameRe.MatchString(s):
				m := renameRe.FindStringSubmatch(s)
				what := "the table " + m[1]
				if m[2] != "" {
					what = "the column " + m[1] + "." + m[2]
				}
				if !st.safe && !newTable(m[1]) {
					add(st.line, Warning, "renames %s to %s: the version running during the deploy uses the old name. Add the new one, copy, and drop the old one a release later; or mark this statement -- safe: <why>", what, m[3])
				}
			case addColumnRe.MatchString(s):
				m := addColumnRe.FindStringSubmatch(s)
				if notNullRe.MatchString(m[3]) && !defaultRe.MatchString(m[3]) && !newTable(m[1]) {
					add(st.line, Error, "adds %s.%s as NOT NULL without a DEFAULT, which fails once %s has rows: give it a DEFAULT", m[1], m[2], m[1])
				}
			case alterTypeRe.MatchString(s) && opts.Dialect == sqldb.Postgres:
				m := alterTypeRe.FindStringSubmatch(s)
				if !st.safe && !newTable(m[1]) {
					add(st.line, Warning, "changes the type of %s.%s, which rewrites %s and locks it until done; on a big table, add a column and copy instead, or mark this statement -- safe: <why>", m[1], m[2], m[1])
				}
			case truncateRe.MatchString(s):
				if !st.safe {
					add(st.line, Warning, "empties a table; mark this statement -- safe: <why> if that's the point")
				}
			case updateRe.MatchString(s) && !whereRe.MatchString(s):
				if !st.safe {
					add(st.line, Warning, "changes every row of %s, with no WHERE; mark this statement -- safe: <why> if that's the point", updateRe.FindStringSubmatch(s)[1])
				}
			case deleteRe.MatchString(s) && !whereRe.MatchString(s):
				if !st.safe {
					add(st.line, Warning, "deletes every row of %s, with no WHERE; mark this statement -- safe: <why> if that's the point", deleteRe.FindStringSubmatch(s)[1])
				}
			}
		}
		if strings.TrimSpace(stripComments(m.down)) == "" {
			add(1, Note, "has no -- +down section, so migrate.Down stops at it")
		}
	}
	if opts.DB != nil {
		out = append(out, roundTrip(ctx, opts.DB, ms)...)
	}
	return out, nil
}

// roundTrip runs every migration up, down and up again in db, an empty
// SQLite database.
func roundTrip(ctx context.Context, db *sql.DB, ms []Migration) []Problem {
	var out []Problem
	exec := func(m Migration, src, what string) bool {
		if strings.TrimSpace(stripComments(src)) == "" {
			return true
		}
		if _, err := db.ExecContext(ctx, src); err != nil {
			out = append(out, Problem{File: m.file, Line: 1, Level: Error, Message: what + ": " + err.Error()})
			return false
		}
		return true
	}
	for _, m := range ms {
		if !exec(m, m.up, "fails on an empty database") {
			return out
		}
	}
	for i := len(ms) - 1; i >= 0; i-- {
		if strings.TrimSpace(stripComments(ms[i].down)) == "" {
			return out // Down stops here too.
		}
		if !exec(ms[i], ms[i].down, "its -- +down section fails") {
			return out
		}
	}
	for _, m := range ms {
		if !exec(m, m.up, "fails after its -- +down section ran, which doesn't undo it all") {
			return out
		}
	}
	return out
}

// splitStatements splits SQL at semicolons outside quotes and comments.
// Line numbers count from the first line of src.
func splitStatements(src string) []statement {
	var out []statement
	var b strings.Builder
	line, start := 1, 0
	safe := false
	flush := func() {
		if s := strings.TrimSpace(b.String()); s != "" {
			out = append(out, statement{sql: s, line: start, safe: safe})
		}
		b.Reset()
		start, safe = 0, false
	}
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\n':
			line++
			b.WriteByte(c)
		case c == '-' && i+1 < len(src) && src[i+1] == '-':
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				j = len(src) - i
			}
			if strings.HasPrefix(strings.TrimSpace(src[i+2:i+j]), "safe:") {
				safe = true
			}
			i += j - 1
		case c == '\'' || c == '"':
			if start == 0 {
				start = line
			}
			j := i + 1
			for j < len(src) && src[j] != c {
				if src[j] == '\n' {
					line++
				}
				j++
			}
			b.WriteString(src[i:min(j+1, len(src))])
			i = j
		case c == ';':
			flush()
		default:
			if start == 0 && c != ' ' && c != '\t' && c != '\r' {
				start = line
			}
			b.WriteByte(c)
		}
	}
	flush()
	return out
}

// stripComments removes -- comments, to tell an empty section.
func stripComments(src string) string {
	var b strings.Builder
	for _, l := range strings.Split(src, "\n") {
		if i := strings.Index(l, "--"); i >= 0 {
			l = l[:i]
		}
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}
