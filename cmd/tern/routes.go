package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/Unfield/Tern/internal/routes"
)

const defaultRoutesDir = "src/routes"

// cmdRoutes prints the routes found in a routes directory.
func cmdRoutes(args []string, stdout, stderr io.Writer) int {
	dir := defaultRoutesDir
	switch len(args) {
	case 0:
	case 1:
		dir = args[0]
	default:
		fmt.Fprintln(stderr, "usage: tern routes [dir]")
		return 2
	}

	m, err := routes.Scan(os.DirFS("."), dir)
	if err != nil {
		fmt.Fprintf(stderr, "tern routes: %v\n", err)
		return 1
	}
	if len(m.Routes) == 0 {
		fmt.Fprintf(stdout, "No routes found in %s\n", dir)
		return 0
	}

	// Show routes alphabetically by URL; matching order is an internal detail.
	list := slices.Clone(m.Routes)
	slices.SortFunc(list, func(a, b *routes.Route) int { return strings.Compare(a.Pattern(), b.Pattern()) })

	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ROUTE\tKIND\tLOAD\tLAYOUTS\tDIRECTORY")
	for _, r := range list {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", r.Pattern(), kind(r), check(r.PageServer != ""), len(r.Layouts), r.ID)
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(stderr, "tern routes: %v\n", err)
		return 1
	}
	return 0
}

func kind(r *routes.Route) string {
	switch {
	case r.Page != "" && r.Endpoint != "":
		return "page+endpoint"
	case r.Page != "":
		return "page"
	default:
		return "endpoint"
	}
}

func check(b bool) string {
	if b {
		return "yes"
	}
	return "-"
}
