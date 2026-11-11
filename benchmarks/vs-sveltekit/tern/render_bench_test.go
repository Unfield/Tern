package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// The benchmarks render a page each time, past the render cache, in this
// process: the load, the payload and the render. benchmarks/compare.sh
// runs them in CI against the commit before.

func BenchmarkItems(b *testing.B) { benchPage(b, "/items") }

func BenchmarkHome(b *testing.B) { benchPage(b, "/") }

func BenchmarkItemsData(b *testing.B) { benchPage(b, "/items/__data.json") }

func benchPage(b *testing.B, path string) {
	app, err := ternApp()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path+"?u="+strconv.Itoa(i), nil))
		if rec.Code != http.StatusOK {
			b.Fatal(rec.Code)
		}
	}
}
