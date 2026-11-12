package search_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/Unfield/Tern/internal/dbtest"
	"github.com/Unfield/Tern/search"
)

var opts = search.Options{Fields: []search.Field{{Name: "title", Boost: 3}, {Name: "body"}}}

// each runs fn on every kind of index.
func each(t *testing.T, fn func(t *testing.T, idx *search.Index)) {
	t.Run("memory", func(t *testing.T) {
		idx, err := search.NewMemory(opts)
		if err != nil {
			t.Fatal(err)
		}
		fn(t, idx)
	})
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		idx, err := search.New(db, "notes", opts)
		if err != nil {
			t.Fatal(err)
		}
		if err := idx.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := idx.Migrate(context.Background()); err != nil {
			t.Fatal("a second Migrate:", err)
		}
		fn(t, idx)
	})
}

func text(segs []search.Segment) string {
	var b strings.Builder
	for _, s := range segs {
		if s.Match {
			b.WriteString("[" + s.Text + "]")
		} else {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

func ids(r search.Results) string {
	var out []string
	for _, h := range r.Hits {
		out = append(out, h.ID)
	}
	return strings.Join(out, ",")
}

func TestSearch(t *testing.T) {
	each(t, func(t *testing.T, idx *search.Index) {
		ctx := context.Background()
		docs := []search.Doc{
			{ID: "1", Scope: "ada", Fields: map[string]string{"title": "Crème brûlée", "body": "A dessert of custard with a layer of hard caramel on top."}},
			{ID: "2", Scope: "ada", Fields: map[string]string{"title": "Caramel sauce", "body": "Sugar, butter and cream, cooked until golden."}},
			{ID: "3", Scope: "bob", Fields: map[string]string{"title": "Bread", "body": "Flour, water, salt and yeast. " + strings.Repeat("Knead it well and let it rest. ", 6) + "Bake it with caramel."}},
			{ID: "4", Scope: "", Fields: map[string]string{"title": "Notes", "body": "Nothing about desserts at all"}},
		}
		for _, d := range docs {
			if err := idx.Put(ctx, d); err != nil {
				t.Fatal(err)
			}
		}

		// A word in a title counts for more than in a body.
		r, err := idx.Search(ctx, search.Query{Text: "caramel"})
		if err != nil {
			t.Fatal(err)
		}
		if r.Total != 3 || r.Hits[0].ID != "2" {
			t.Errorf("caramel = %s (%d)", ids(r), r.Total)
		}
		// Snippets mark the matches; long fields are cut around them.
		if got := text(r.Hits[0].Snippets["title"]); got != "[Caramel] sauce" {
			t.Errorf("title snippet = %q", got)
		}
		for _, h := range r.Hits {
			if h.ID == "3" {
				body := text(h.Snippets["body"])
				if !strings.Contains(body, "[caramel]") || !strings.HasPrefix(body, "…") || len(body) > 200 {
					t.Errorf("long snippet = %q", body)
				}
			}
		}

		// Prefixes match as someone types, without accents, and every word must.
		for q, want := range map[string]string{
			"cara":           "2,1,3",
			"creme":          "1",
			"CRÈME brû":      "1",
			"caramel butter": "2",
			"caramel nope":   "",
			`"OR" AND (`:     "",
			"":               "",
		} {
			r, err := idx.Search(ctx, search.Query{Text: q})
			if err != nil {
				t.Fatalf("%q: %v", q, err)
			}
			if got := ids(r); !sameSet(got, want) {
				t.Errorf("%q = %q, want %q", q, got, want)
			}
		}

		// A scope finds only its documents.
		if r, _ := idx.Search(ctx, search.Query{Text: "caramel", Scope: "bob"}); ids(r) != "3" {
			t.Errorf("bob's caramel = %s", ids(r))
		}

		// Pages.
		r, _ = idx.Search(ctx, search.Query{Text: "caramel", Limit: 2})
		r2, _ := idx.Search(ctx, search.Query{Text: "caramel", Limit: 2, Offset: 2})
		if len(r.Hits) != 2 || len(r2.Hits) != 1 || r2.Total != 3 {
			t.Errorf("pages: %s / %s", ids(r), ids(r2))
		}

		// Put replaces; Delete and Reset remove.
		if err := idx.Put(ctx, search.Doc{ID: "2", Scope: "ada", Fields: map[string]string{"title": "Toffee", "body": "Butter and sugar."}}); err != nil {
			t.Fatal(err)
		}
		if r, _ := idx.Search(ctx, search.Query{Text: "caramel sauce"}); r.Total != 0 {
			t.Errorf("replaced doc still found: %s", ids(r))
		}
		if r, _ := idx.Search(ctx, search.Query{Text: "toffee"}); ids(r) != "2" {
			t.Errorf("new text not found: %s", ids(r))
		}
		if err := idx.Delete(ctx, "1"); err != nil {
			t.Fatal(err)
		}
		if r, _ := idx.Search(ctx, search.Query{Text: "creme"}); r.Total != 0 {
			t.Error("deleted doc found")
		}
		if err := idx.Reset(ctx); err != nil {
			t.Fatal(err)
		}
		if r, _ := idx.Search(ctx, search.Query{Text: "bread"}); r.Total != 0 {
			t.Error("Reset left documents")
		}
	})
}

func sameSet(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	as, bs := strings.Split(a, ","), strings.Split(b, ",")
	if len(as) != len(bs) {
		return false
	}
	seen := map[string]bool{}
	for _, x := range as {
		seen[x] = true
	}
	for _, x := range bs {
		if !seen[x] {
			return false
		}
	}
	return true
}

func TestBadIndexes(t *testing.T) {
	for _, o := range []search.Options{
		{},
		{Fields: []search.Field{{Name: "id"}}},
		{Fields: []search.Field{{Name: "a b"}}},
		{Fields: []search.Field{{Name: "a"}, {Name: "A"}}},
		{Fields: []search.Field{{Name: "a"}}, Language: "english; DROP"},
	} {
		if _, err := search.NewMemory(o); err == nil {
			t.Errorf("%+v: no error", o)
		}
	}
	if _, err := search.NewMemory(search.Options{Fields: []search.Field{{Name: "a"}}}); err != nil {
		t.Error(err)
	}
	if err := func() error {
		idx, _ := search.NewMemory(opts)
		return idx.Put(context.Background(), search.Doc{})
	}(); err == nil {
		t.Error("a document without an ID was put")
	}
}

func TestWords(t *testing.T) {
	if got := fmt.Sprint(search.Words("Crème Brûlée, o'Neil: 3.5% — Ünïcode")); got != "[creme brulee o neil 3 5 unicode]" {
		t.Errorf("Words = %s", got)
	}
}

func TestText(t *testing.T) {
	if got := search.Text("<h2>Install</h2><p>Run <code>go get</code>, then <strong>build</strong>.</p>"); !strings.Contains(got, "Install") || !strings.Contains(got, "Run go get, then build.") || strings.Contains(got, "<") {
		t.Errorf("Text = %q", got)
	}
}
