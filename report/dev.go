package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/internal/batch"
)

// Under tern dev, reports go to the dev server's log view, which lists
// them by group. One shipper per process.
var dev struct {
	once sync.Once
	sh   *batch.Shipper[Report]
}

func devShipper() *batch.Shipper[Report] {
	dev.once.Do(func() {
		u := os.Getenv("TERN_DEV_LOGS")
		if u == "" {
			return
		}
		u = strings.TrimSuffix(u, "/ingest") + "/ingest-errors"
		token := os.Getenv("TERN_DEV_LOGS_TOKEN")
		client := &http.Client{Timeout: 2 * time.Second}
		send := func(ctx context.Context, reports []Report) error {
			body, err := json.Marshal(reports)
			if err != nil {
				return batch.Permanent(err)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
			if err != nil {
				return batch.Permanent(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				return batch.Permanent(fmt.Errorf("tern dev answered %s", resp.Status))
			}
			return nil
		}
		dev.sh = batch.New(batch.Config{Name: "tern dev"}, send, batch.Options{Size: 64, Interval: 100 * time.Millisecond, MaxRetries: 2, Timeout: 2 * time.Second})
	})
	return dev.sh
}

// DevOnly returns a reporter that only feeds tern dev's log view, or nil
// outside tern dev. kit uses it when the app has no reporter of its own.
func DevOnly() *Reporter {
	if devShipper() == nil {
		return nil
	}
	r := New(Options{})
	return r
}
