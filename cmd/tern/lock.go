package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// lockDev makes sure only one `tern dev` runs per project: two would
// overwrite each other's generated files and server binary. It returns a
// function that releases the lock.
func lockDev(root string) (func(), error) {
	dir := filepath.Join(root, ".tern", "dev")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "tern-dev.pid")
	for range 2 {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		b, _ := os.ReadFile(path)
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 && pid != os.Getpid() && processAlive(pid) {
			return nil, fmt.Errorf("tern dev is already running for this project (process %d); stop it first", pid)
		}
		_ = os.Remove(path) // stale lock from a crashed run
	}
	return nil, errors.New("could not take the tern dev lock")
}
