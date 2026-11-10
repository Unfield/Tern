package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/Unfield/Tern/internal/routes"
	"github.com/Unfield/Tern/internal/ui"
)

const defaultRoutesDir = "src/routes"

// cmdRoutes prints the routes found in a routes directory.
func cmdRoutes(args []string, stdout, stderr io.Writer) int {
	dir := defaultRoutesDir
	jsonOut := false
	var positional []string
	for _, a := range args {
		switch a {
		case "-json", "--json":
			jsonOut = true
		default:
			positional = append(positional, a)
		}
	}
	switch len(positional) {
	case 0:
	case 1:
		dir = positional[0]
	default:
		fmt.Fprintln(stderr, "usage: tern routes [dir] [-json]")
		return 2
	}

	m, err := routes.Scan(os.DirFS("."), dir)
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	p := ui.New(stdout)
	if len(m.Routes) == 0 {
		if jsonOut {
			_, _ = stdout.Write([]byte("[]\n"))
		} else {
			p.Printf("No routes found in %s\n", dir)
		}
		return 0
	}

	// Show routes alphabetically by URL; matching order is an internal detail.
	list := slices.Clone(m.Routes)
	slices.SortFunc(list, func(a, b *routes.Route) int { return strings.Compare(a.Pattern(), b.Pattern()) })

	if jsonOut {
		type routeJSON struct {
			Pattern string `json:"pattern"`
			ID      string `json:"id"`
			Kind    string `json:"kind"`
			Load    bool   `json:"load"`
			Layouts int    `json:"layouts"`
		}
		out := make([]routeJSON, 0, len(list))
		for _, r := range list {
			out = append(out, routeJSON{
				Pattern: r.Pattern(),
				ID:      r.ID,
				Kind:    kind(r),
				Load:    r.HasLoad(),
				Layouts: len(r.Layouts),
			})
		}
		enc := json.NewEncoder(stdout)
		if err := enc.Encode(out); err != nil {
			ui.New(stderr).Error(err)
			return 1
		}
		return 0
	}

	rows := [][]string{{"ROUTE", "KIND", "LOAD", "LAYOUTS", "DIRECTORY"}}
	for _, r := range list {
		rows = append(rows, []string{r.Pattern(), kind(r), check(r.HasLoad()), strconv.Itoa(len(r.Layouts)), r.ID})
	}
	p.Println()
	p.Table(rows, func(col int, cell string) string {
		switch col {
		case 0:
			return p.Bold(cell)
		case 1:
			if cell == "endpoint" {
				return p.Yellow(cell)
			}
			return p.Cyan(cell)
		case 2:
			if cell == "yes" {
				return p.Green(cell)
			}
			return p.Dim(cell)
		case 4:
			return p.Dim(cell)
		}
		return cell
	})
	p.Printf("\n  %s\n\n", p.Dim(plural(len(list), "route")))
	return 0
}

func kind(r *routes.Route) string {
	switch {
	case r.Page != "" && r.IsEndpoint():
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
