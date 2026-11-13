package shelfapp

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// Start is what the phone app calls: the app on a loopback port, with its
// database in the app's data directory.
func TestStart(t *testing.T) {
	addr, err := Start(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer Stop()
	if !strings.HasPrefix(addr, "http://127.0.0.1:") {
		t.Fatalf("address %q", addr)
	}
	if again, _ := Start(t.TempDir()); again != addr {
		t.Errorf("a second Start: %q, want %q", again, addr)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Get(addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), "/login") {
		t.Errorf("signed out: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	res, err = client.Get(addr + "/login")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "data-tern-ssr") {
		t.Errorf("the sign-in page isn't rendered on the device: %d", res.StatusCode)
	}
}
