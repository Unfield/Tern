// Package shelfapp is Shelf as a library, for a phone app to run on the
// device: the spike in mobile/SPIKE.md. cmd/shelf runs it as a program.
package shelfapp

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/migrate"
	"github.com/Unfield/Tern/report"

	"shelf/internal/shelf"
	"shelf/migrations"
)

// newApp opens the database at path and returns the app. The tests call
// it too, each with a database of its own.
func newApp(path string) (*kit.App, error) {
	if err := shelf.Open(context.Background(), path); err != nil {
		return nil, err
	}
	provider, server, err := assistants()
	if err != nil {
		return nil, err
	}
	reporter := report.New(report.Options{})
	panel, err := adminPanel(reporter)
	if err != nil {
		return nil, err
	}
	return ternApp(kit.WithReporter(reporter), shelf.Accounts.Guard(), kit.WithPlugins(
		migrate.Plugin(shelf.DB, migrations.FS),
		reindex{},
		shelf.Accounts,
		shelf.Flags,
		provider,
		server,
		panel,
	))
}

// reindex indexes the links for search when the app starts.
type reindex struct{}

func (reindex) Name() string { return "reindex" }

func (reindex) Setup(s *kit.Setup) error {
	s.OnStart(shelf.Reindex)
	return nil
}

// Run serves the app on its usual address, with the database at path,
// until it fails.
func Run(path string) error {
	app, err := newApp(path)
	if err != nil {
		return err
	}
	return app.ListenAndServe()
}

var (
	mu      sync.Mutex
	running *http.Server
	address string
)

// Start runs the app on the device: its database in dataDir, served on a
// port of the loopback interface that only this device can reach. It
// returns the address for the web view, such as "http://127.0.0.1:52341".
// Calling it again returns the same address. Its signature is what gomobile
// can bind: strings and an error.
func Start(dataDir string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if running != nil {
		return address, nil
	}
	app, err := newApp(filepath.Join(dataDir, "shelf.db"))
	if err != nil {
		return "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	if err := app.Start(); err != nil {
		_ = ln.Close()
		return "", err
	}
	srv := &http.Server{Handler: app, ReadHeaderTimeout: 10 * time.Second}
	app.ConfigureServer(srv)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("shelf: serving", "error", err)
		}
	}()
	running, address = srv, "http://"+ln.Addr().String()
	return address, nil
}

// Stop shuts the app down, when the phone app is closed.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if running == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = running.Shutdown(ctx)
	running, address = nil, ""
}
