package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPlainOutputWhenNotATerminal(t *testing.T) {
	t.Setenv("FORCE_COLOR", "")
	var buf bytes.Buffer
	p := New(&buf)
	p.Success("built %s", "app")
	s := p.Spin("compiling")
	s.Done("3 routes")
	p.Error(errors.New("first line\nsecond line"))
	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("output to a non-terminal must not contain escape codes: %q", out)
	}
	for _, want := range []string{"✓ built app", "✓ compiling 3 routes · ", "error first line", "      second line"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestForceColor(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	var buf bytes.Buffer
	if got := New(&buf).Green("ok"); got != "\x1b[32mok\x1b[0m" {
		t.Errorf("Green = %q", got)
	}
}

func TestTable(t *testing.T) {
	t.Setenv("FORCE_COLOR", "")
	var buf bytes.Buffer
	New(&buf).Table([][]string{{"ROUTE", "KIND"}, {"/", "page"}, {"/api/stats", "endpoint"}}, nil)
	want := "  ROUTE       KIND\n  /           page\n  /api/stats  endpoint\n"
	if buf.String() != want {
		t.Errorf("table:\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestFormatting(t *testing.T) {
	if Duration(812*time.Millisecond) != "812ms" || Duration(1400*time.Millisecond) != "1.4s" {
		t.Error("Duration")
	}
	if Size(512) != "512 B" || Size(14_200) != "14.2 kB" || Size(10_300_000) != "10.3 MB" {
		t.Error("Size")
	}
}
