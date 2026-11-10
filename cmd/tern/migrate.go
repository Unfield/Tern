package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Unfield/Tern/internal/ui"
)

// migrationsDir is where tern migrate new writes migrations, next to the
// app's main package, which embeds them.
const migrationsDir = "migrations"

var migrationNameRe = regexp.MustCompile(`[^a-z0-9]+`)

// cmdMigrate creates migration files. Applying them happens in the app
// (migrate.Plugin), which has the database connection.
func cmdMigrate(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || args[0] != "new" {
		fmt.Fprintln(stderr, "usage: tern migrate new <name>")
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
