package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// BenchmarkItems renders /items each time, past the render cache, in this
// process: the load, the payload and the render.
func BenchmarkItems(b *testing.B) {
	app, err := ternApp()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/items?u="+strconv.Itoa(i), nil))
		if rec.Code != 200 {
			b.Fatal(rec.Code)
		}
	}
}
