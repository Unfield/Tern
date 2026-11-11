package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Unfield/Tern/internal/batch"
)

func TestStatuses(t *testing.T) {
	status, retry := 0, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if retry != "" {
			w.Header().Set("Retry-After", retry)
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	for _, tc := range []struct {
		status    int
		retry     string
		permanent bool
		ok        bool
	}{
		{status: 204, ok: true},
		{status: 400, permanent: true},
		{status: 401, permanent: true},
		{status: 408},
		{status: 429, retry: "7"},
		{status: 503},
	} {
		status, retry = tc.status, tc.retry
		err := Do(context.Background(), Request{URL: srv.URL, ContentType: "application/json", Body: []byte("{}"), Gzip: true})
		if (err == nil) != tc.ok || batch.IsPermanent(err) != tc.permanent {
			t.Errorf("%d: %v", tc.status, err)
		}
	}
	if d := retryAfter("7"); d != 7*time.Second {
		t.Error(d)
	}
}

func TestParsePairs(t *testing.T) {
	got := ParsePairs(" a=1, b = x%20y ,bad, =z,c=")
	if len(got) != 3 || got[0] != (Pair{"a", "1"}) || got[1] != (Pair{"b", "x y"}) || got[2] != (Pair{"c", ""}) {
		t.Fatal(got)
	}
}
