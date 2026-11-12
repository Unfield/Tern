// Package sqlgen turns an app's queries/*.sql files into a Go package of
// typed functions (ADR 0028). Types come from the schema the app's
// migrations build, in a scratch SQLite database.
package sqlgen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Kinds of queries, from the "-- name: X :kind" line.
const (
	kindOne      = "one"
	kindMany     = "many"
	kindExec     = "exec"
	kindExecRows = "execrows"
)

// query is one named query from a .sql file.
type query struct {
	name, kind string
	file       string
	line       int // of the "-- name:" line
	doc        []string
	raw        string // as written, with :name parameters
	sql        string // with ? for each parameter
	// params in order of first use; uses is every use, in order, as an
	// index into params.
	params []*param
	uses   []int
	// declared types from "-- param" and "-- column" lines, and the row
	// type's name from "-- row".
	paramTypes  map[string]string
	columnTypes map[string]string
	columnOrder []string
	row         string
	unchecked   bool

	columns []column // filled in by typing
}

type param struct {
	name   string // as written, such as user_id
	goType string
}

type column struct {
	name   string
	goType string
}

var (
	nameRe   = regexp.MustCompile(`^--\s*name:\s*([A-Za-z][A-Za-z0-9_]*)\s*:(one|many|exec|execrows)\s*$`)
	paramRe  = regexp.MustCompile(`^--\s*param\s+([A-Za-z_][A-Za-z0-9_]*)\s+(\S+)\s*$`)
	columnRe = regexp.MustCompile(`^--\s*column\s+([A-Za-z_][A-Za-z0-9_]*)\s+(\S+)\s*$`)
	rowRe    = regexp.MustCompile(`^--\s*row\s+([A-Z][A-Za-z0-9_]*)\s*$`)
)

// goTypes are the types a "-- param" or "-- column" line may name, with
// or without a * for NULL.
var goTypes = []string{"int64", "int", "float64", "string", "bool", "[]byte", "time.Time", "json.RawMessage", "sealed.String", "sealed.Bytes"}

// parseDir reads every .sql file in dir.
func parseDir(dir string) ([]*query, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	var all []*query
	var errs []string
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		qs, fileErrs := parseFile(filepath.ToSlash(f), string(src))
		all = append(all, qs...)
		errs = append(errs, fileErrs...)
	}
	seen := map[string]*query{}
	for _, q := range all {
		if other, ok := seen[q.name]; ok {
			errs = append(errs, fmt.Sprintf("%s:%d: query %s is also defined at %s:%d", q.file, q.line, q.name, other.file, other.line))
		}
		seen[q.name] = q
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return all, nil
}

// parseFile splits a file into its named queries.
func parseFile(file, src string) ([]*query, []string) {
	var out []*query
	var errs []string
	var cur *query
	var body []string
	var pending []string // comments before a "-- name:" line
	finish := func() {
		if cur == nil {
			return
		}
		// Comments after the SQL document the next query.
		end := len(body)
		for end > 0 {
			t := strings.TrimSpace(body[end-1])
			if t != "" && !strings.HasPrefix(t, "--") {
				break
			}
			end--
		}
		for _, line := range body[end:] {
			if text, ok := strings.CutPrefix(strings.TrimSpace(line), "--"); ok {
				pending = append(pending, strings.TrimSpace(text))
			} else {
				pending = nil
			}
		}
		body = body[:end]
		cur.sql = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.Join(body, "\n")), ";"))
		cur.raw = cur.sql
		if cur.sql == "" {
			errs = append(errs, fmt.Sprintf("%s:%d: query %s has no SQL", file, cur.line, cur.name))
		} else if err := cur.parseParams(); err != nil {
			errs = append(errs, fmt.Sprintf("%s:%d: %s: %v", file, cur.line, cur.name, err))
		}
		out = append(out, cur)
		cur, body = nil, nil
	}
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if m := nameRe.FindStringSubmatch(trimmed); m != nil {
			finish()
			cur = &query{name: m[1], kind: m[2], file: file, line: i + 1, doc: pending,
				paramTypes: map[string]string{}, columnTypes: map[string]string{}}
			pending = nil
			continue
		}
		if strings.HasPrefix(trimmed, "-- name:") {
			errs = append(errs, fmt.Sprintf("%s:%d: write the name line as -- name: QueryName :one (or :many, :exec, :execrows)", file, i+1))
			continue
		}
		if cur == nil {
			if text, ok := strings.CutPrefix(trimmed, "--"); ok {
				pending = append(pending, strings.TrimSpace(text))
			} else if trimmed == "" {
				pending = nil
			} else {
				errs = append(errs, fmt.Sprintf("%s:%d: SQL before the first -- name: line", file, i+1))
			}
			continue
		}
		if len(body) == 0 && strings.HasPrefix(trimmed, "--") {
			switch {
			case paramRe.MatchString(trimmed):
				m := paramRe.FindStringSubmatch(trimmed)
				if !validType(m[2]) {
					errs = append(errs, fmt.Sprintf("%s:%d: %s isn't a type a parameter can have: %s", file, i+1, m[2], strings.Join(goTypes, ", ")))
				}
				cur.paramTypes[m[1]] = m[2]
			case columnRe.MatchString(trimmed):
				m := columnRe.FindStringSubmatch(trimmed)
				if !validType(m[2]) {
					errs = append(errs, fmt.Sprintf("%s:%d: %s isn't a type a column can have: %s", file, i+1, m[2], strings.Join(goTypes, ", ")))
				}
				cur.columnTypes[m[1]] = m[2]
				cur.columnOrder = append(cur.columnOrder, m[1])
			case rowRe.MatchString(trimmed):
				cur.row = rowRe.FindStringSubmatch(trimmed)[1]
			case trimmed == "-- unchecked":
				cur.unchecked = true
			default:
				cur.doc = append(cur.doc, strings.TrimSpace(strings.TrimPrefix(trimmed, "--")))
			}
			continue
		}
		body = append(body, line)
	}
	finish()
	return out, errs
}

func validType(t string) bool { return slices.Contains(goTypes, strings.TrimPrefix(t, "*")) }

// parseParams finds the :name parameters outside strings, quoted names
// and comments, and replaces each use with ?.
func (q *query) parseParams() error {
	var b strings.Builder
	s := q.sql
	index := map[string]int{}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			j := i + 1
			for j < len(s) && s[j] != c {
				j++
			}
			b.WriteString(s[i:min(j+1, len(s))])
			i = j
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				j = len(s) - i
			}
			b.WriteString(s[i : i+j])
			i += j - 1
		case c == ':' && i+1 < len(s) && s[i+1] == ':':
			b.WriteString("::") // a PostgreSQL cast
			i++
		case c == ':' && i+1 < len(s) && isIdentStart(s[i+1]):
			j := i + 1
			for j < len(s) && isIdent(s[j]) {
				j++
			}
			name := s[i+1 : j]
			n, ok := index[name]
			if !ok {
				n = len(q.params)
				index[name] = n
				q.params = append(q.params, &param{name: name})
			}
			q.uses = append(q.uses, n)
			b.WriteByte('?')
			i = j - 1
		case c == '?' || c == '$' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			return fmt.Errorf("use named parameters, :name, instead of %c", c)
		default:
			b.WriteByte(c)
		}
	}
	for name := range q.paramTypes {
		if _, ok := index[name]; !ok {
			return fmt.Errorf("-- param %s: the query has no :%s", name, name)
		}
	}
	q.sql = b.String()
	return nil
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isIdent(c byte) bool { return isIdentStart(c) || c >= '0' && c <= '9' }
