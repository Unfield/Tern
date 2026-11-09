package main

import (
	"fmt"
	"io"

	"github.com/Unfield/Tern/internal/codegen"
)

// cmdGen regenerates tern.gen.go, the client manifest and route types.
func cmdGen(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "usage: tern gen")
		return 2
	}
	res, err := codegen.Generate(".")
	if err != nil {
		fmt.Fprintf(stderr, "tern gen: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Generated %d routes", len(res.Manifest.Routes))
	if len(res.Changed) > 0 {
		fmt.Fprintf(stdout, " (%d files updated)", len(res.Changed))
	}
	fmt.Fprintln(stdout)
	return 0
}
