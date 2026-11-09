package main

import (
	"fmt"
	"io"
	"time"

	"github.com/Unfield/Tern/internal/codegen"
	"github.com/Unfield/Tern/internal/ui"
)

// cmdGen regenerates tern.gen.go, the client manifest and route types.
func cmdGen(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "usage: tern gen")
		return 2
	}
	start := time.Now()
	res, err := codegen.Generate(".")
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	p := ui.New(stdout)
	note := "up to date"
	if n := len(res.Changed); n > 0 {
		note = plural(n, "file") + " updated"
	}
	p.Success("Generated %s %s", plural(len(res.Manifest.Routes), "route"), p.Dim(note+" · "+ui.Duration(time.Since(start))))
	return 0
}
