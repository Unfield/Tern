// Package routes is the root layout and the landing page.
package routes

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Unfield/Tern/kit"

	"github.com/Unfield/Tern/website/internal/docs"
)

const Prerender = true

// Sample is a highlighted file.
type Sample struct {
	File string `json:"file"`
	HTML string `json:"html"`
	Code string `json:"code"`
}

// Scenario is one benchmark, in requests per second for each server.
type Scenario struct {
	Name    string    `json:"name"`
	Servers []string  `json:"servers"`
	Values  []float64 `json:"values"`
}

type Home struct {
	Hero       []Sample   `json:"hero"`
	Types      []Sample   `json:"types"`
	Validation Sample     `json:"validation"`
	Benchmarks []Scenario `json:"benchmarks"`
}

const loadGo = `package post

import "github.com/Unfield/Tern/kit"

type Post struct {
	Title string ` + "`json:\"title\"`" + `
	Body  string ` + "`json:\"body\"`" + `
	Likes int    ` + "`json:\"likes\"`" + `
}

// Load runs on the server, before the page renders.
func Load(c *kit.Ctx) (Post, error) {
	return db.Post(c, c.Param("slug"))
}

var Actions = kit.Actions{
	"like": kit.Action(func(c *kit.Ctx) (Post, error) {
		return db.Like(c, c.Param("slug"))
	}),
}
`

const pageSvelte = `<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import type { PageProps } from './$types';

	// data is a Post, typed from the Go struct.
	let { data }: PageProps = $props();
</script>

<article>
	<h1>{data.title}</h1>
	{@html data.body}

	<form method="POST" action="?/like" use:enhance>
		<button>♥ {data.likes}</button>
	</form>
</article>
`

const typesGo = `type Invoice struct {
	ID     string     ` + "`json:\"id\"`" + `
	Amount int64      ` + "`json:\"amount\"`" + `
	Paid   *time.Time ` + "`json:\"paid,omitempty\"`" + `
	Lines  []Line     ` + "`json:\"lines\"`" + `
}
`

const typesTS = `// .tern/types: generated from the Go struct
export interface Invoice {
	id: string;
	amount: number;
	paid?: string;
	lines: Line[];
}
`

const validationGo = `type SignUp struct {
	Name     string ` + "`form:\"name\" validate:\"required,max=80\"`" + `
	Email    string ` + "`form:\"email\" validate:\"required,email\"`" + `
	Password string ` + "`form:\"password\" validate:\"required,min=10\"`" + `
}
`

func sample(file, lang, code string) Sample {
	return Sample{File: file, HTML: docs.Highlight(lang, code), Code: code}
}

func Load(ctx *kit.Ctx) (Home, error) {
	h := Home{
		Hero: []Sample{
			sample("src/routes/blog/slug.param/page.server.go", "go", loadGo),
			sample("src/routes/blog/slug.param/+page.svelte", "svelte", pageSvelte),
		},
		Types:      []Sample{sample("invoice.go", "go", typesGo), sample("$types.d.ts", "typescript", typesTS)},
		Validation: sample("signup.go", "go", validationGo),
	}
	b, err := os.ReadFile(filepath.Join(docs.Root, "docs", "benchmarks.md"))
	if err != nil {
		return h, err
	}
	h.Benchmarks = benchmarks(b)
	return h, nil
}

// benchmarks reads the "Compared with SvelteKit" table of
// docs/benchmarks.md, so the chart shows what the docs say. Rows given as
// ranges, and memory, are left out.
func benchmarks(md []byte) []Scenario {
	_, table, ok := bytes.Cut(md, []byte("## Compared with SvelteKit"))
	if !ok {
		return nil
	}
	var servers []string
	var out []Scenario
	sc := bufio.NewScanner(bytes.NewReader(table))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "## ") {
			break
		}
		if !strings.HasPrefix(line, "|") {
			if len(servers) > 0 && line == "" && len(out) > 0 {
				break
			}
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if servers == nil {
			servers = cells[1:]
			continue
		}
		if strings.HasPrefix(cells[0], "---") || strings.HasPrefix(cells[0], ":") || cells[0] == "" && strings.HasPrefix(cells[1], "---") {
			continue
		}
		s := Scenario{Name: cells[0], Servers: servers}
		ok := true
		for _, c := range cells[1:] {
			v, err := strconv.ParseFloat(strings.ReplaceAll(c, ",", ""), 64)
			if err != nil {
				ok = false
				break
			}
			s.Values = append(s.Values, v)
		}
		if ok && len(s.Values) == len(servers) {
			out = append(out, s)
		}
	}
	return out
}
