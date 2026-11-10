// Package webhook receives notifications in end-to-end tests: POST stores
// the body, GET returns everything received. It exists only when
// STATUS_TEST_MODE=1.
package webhook

import (
	"encoding/json"
	"io"
	"os"
	"sync"

	"github.com/Unfield/Tern/kit"
)

var (
	mu       sync.Mutex
	received = []json.RawMessage{}
)

func enabled() bool { return os.Getenv("STATUS_TEST_MODE") == "1" }

func GET(ctx *kit.Ctx) error {
	if !enabled() {
		return kit.NotFound()
	}
	mu.Lock()
	defer mu.Unlock()
	return ctx.JSON(200, received)
}

func POST(ctx *kit.Ctx) error {
	if !enabled() {
		return kit.NotFound()
	}
	b, err := io.ReadAll(io.LimitReader(ctx.Request.Body, 1<<20))
	if err != nil {
		return err
	}
	if !json.Valid(b) {
		return kit.Error(400, "not JSON")
	}
	mu.Lock()
	received = append(received, b)
	mu.Unlock()
	return ctx.Text(200, "ok")
}
