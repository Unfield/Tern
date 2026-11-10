// Package target is a service to monitor in end-to-end tests: it answers
// with the status code last POSTed to it. It exists only when
// STATUS_TEST_MODE=1.
package target

import (
	"os"
	"strconv"
	"sync/atomic"

	"github.com/Unfield/Tern/kit"
)

var code atomic.Int32

func init() { code.Store(200) }

func enabled() bool { return os.Getenv("STATUS_TEST_MODE") == "1" }

func GET(ctx *kit.Ctx) error {
	if !enabled() {
		return kit.NotFound()
	}
	c := int(code.Load())
	return ctx.Text(c, "target says "+strconv.Itoa(c))
}

// POST sets the status code, from ?code=500.
func POST(ctx *kit.Ctx) error {
	if !enabled() {
		return kit.NotFound()
	}
	c, err := strconv.Atoi(ctx.Request.URL.Query().Get("code"))
	if err != nil || c < 200 || c > 599 {
		return kit.Error(400, "code must be an HTTP status")
	}
	code.Store(int32(c))
	return ctx.Text(200, "ok")
}
