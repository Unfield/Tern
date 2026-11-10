package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{name: "no args", args: nil, wantCode: 2, wantErr: "Usage"},
		{name: "help", args: []string{"help"}, wantCode: 0, wantOut: "Usage"},
		{name: "version", args: []string{"version"}, wantCode: 0, wantOut: "tern "},
		{name: "unknown", args: []string{"nope"}, wantCode: 2, wantErr: `unknown command "nope"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(stdout.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantOut)
			}
			if !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantErr)
			}
		})
	}
}

func TestMigrateNew(t *testing.T) {
	t.Chdir(t.TempDir())
	var out, errb bytes.Buffer
	if code := run([]string{"migrate", "new", "Add Users", "table!"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	files, _ := filepath.Glob("migrations/*_add_users_table.sql")
	if len(files) != 1 {
		t.Fatalf("files = %v, output %s", files, out.String())
	}
	b, _ := os.ReadFile(files[0])
	if !strings.Contains(string(b), "-- +down") {
		t.Errorf("content = %q", b)
	}
	if code := run([]string{"migrate"}, &out, &errb); code != 2 {
		t.Errorf("usage exit = %d", code)
	}
}
