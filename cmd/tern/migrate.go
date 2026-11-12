package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/internal/ui"
	"github.com/Unfield/Tern/migrate"
	"github.com/Unfield/Tern/sqldb"
)

// migrationsDir is where tern migrate new writes migrations, next to the
// app's main package, which embeds them.
const migrationsDir = "migrations"

var migrationNameRe = regexp.MustCompile(`[^a-z0-9]+`)

// cmdMigrate creates migration files. Applying them happens in the app
// (migrate.Plugin), which has the database connection.
func cmdMigrate(args []string, stdout, stderr io.Writer) int {
	if len(args) >= 1 && args[0] == "check" {
		return cmdMigrateCheck(args[1:], stdout, stderr)
	}
	if len(args) < 2 || args[0] != "new" {
		fmt.Fprintln(stderr, "usage: tern migrate new <name>\n       tern migrate check [-json]")
		return 2
	}
	name := strings.Trim(migrationNameRe.ReplaceAllString(strings.ToLower(strings.Join(args[1:], " ")), "_"), "_")
	if name == "" {
		ui.New(stderr).Error(fmt.Errorf("the migration needs a name made of letters or digits"))
		return 2
	}
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	file := filepath.Join(migrationsDir, time.Now().UTC().Format("20060102150405")+"_"+name+".sql")
	content := "-- " + strings.ReplaceAll(name, "_", " ") + "\n\n\n-- +down\n\n"
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	_, err = f.WriteString(content)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	p := ui.New(stdout)
	p.Printf("  %s %s\n", p.Green("✓"), "Created "+p.Cyan(filepath.ToSlash(file)))
	p.Printf("  %s\n", p.Dim("Apply it with migrate.Plugin(db, migrations) in your app; see docs/guide.md#database-migrations"))
	return 0
}

func cmdMigrateCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("migrate check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print the problems as JSON")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: tern migrate check [-json]\n\nLooks for migrations that fail, or break the version of the app still running while a deploy rolls out: NOT NULL columns without a default, dropped or renamed tables and columns, changes to every row. A SQLite app's migrations also run up, down and up again in a scratch database. It exits 1 when one has an error.")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	root, _ := os.Getwd()
	problems, err := checkMigrations(root)
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	failed := slices.ContainsFunc(problems, func(p migrate.Problem) bool { return p.Level == migrate.Error })
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if problems == nil {
			problems = []migrate.Problem{}
		}
		_ = enc.Encode(problems)
	} else {
		p := ui.New(stdout)
		if len(problems) == 0 {
			p.Printf("  %s %s\n", p.Green("✓"), "The migrations look safe")
		}
		for _, pr := range problems {
			mark := p.Dim("·")
			switch pr.Level {
			case migrate.Error:
				mark = p.Red("✗")
			case migrate.Warning:
				mark = p.Yellow("!")
			}
			p.Printf("  %s %s %s\n", mark, p.Cyan(fmt.Sprintf("%s:%d", path.Join(migrationsDir, pr.File), pr.Line)), pr.Message)
		}
	}
	if failed {
		return 1
	}
	return 0
}

// checkMigrations runs migrate.Check on the app in root: for its
// database, and in a scratch SQLite database when that's what it uses.
func checkMigrations(root string) ([]migrate.Problem, error) {
	dir := filepath.Join(root, migrationsDir)
	if _, err := os.Stat(dir); err != nil {
		return nil, nil
	}
	opts := migrate.CheckOptions{Dialect: appDialect(root)}
	if opts.Dialect == sqldb.SQLite {
		db, err := sql.Open("sqlite", "file:migratecheck-"+strconv.Itoa(os.Getpid())+"?mode=memory&cache=shared")
		if err != nil {
			return nil, err
		}
		defer func() { _ = db.Close() }()
		db.SetMaxOpenConns(1)
		opts.DB = db
	}
	return migrate.Check(context.Background(), os.DirFS(dir), opts)
}

// appDialect is the database the app uses, from the drivers its go.mod
// requires directly: PostgreSQL with pgx or lib/pq and no SQLite driver,
// SQLite otherwise.
func appDialect(root string) sqldb.Dialect {
	mod, _ := os.ReadFile(filepath.Join(root, "go.mod"))
	var postgres, sqlite bool
	for line := range strings.SplitSeq(string(mod), "\n") {
		if strings.Contains(line, "// indirect") {
			continue
		}
		switch {
		case strings.Contains(line, "github.com/jackc/pgx"), strings.Contains(line, "github.com/lib/pq"):
			postgres = true
		case strings.Contains(line, "modernc.org/sqlite"), strings.Contains(line, "github.com/mattn/go-sqlite3"):
			sqlite = true
		}
	}
	if postgres && !sqlite {
		return sqldb.Postgres
	}
	return sqldb.SQLite
}
