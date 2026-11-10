// Command load sends requests to a URL from a fixed number of connections
// for a while, and prints throughput and latency as one JSON line.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	url := flag.String("url", "", "the URL to load")
	conns := flag.Int("c", 32, "concurrent connections")
	dur := flag.Duration("d", 10*time.Second, "how long to measure")
	warm := flag.Duration("warmup", 3*time.Second, "how long to warm up first")
	unique := flag.Bool("unique", false, "add ?r=<n> to every URL, so no cache can answer")
	flag.Parse()
	if *url == "" {
		fmt.Fprintln(os.Stderr, "usage: load -url http://localhost:3000/ [-c 32] [-d 10s] [-unique]")
		os.Exit(2)
	}
	client := &http.Client{Transport: &http.Transport{MaxIdleConns: *conns, MaxIdleConnsPerHost: *conns, DisableCompression: true}, Timeout: 10 * time.Second}

	var seq atomic.Int64
	target := func() string {
		if !*unique {
			return *url
		}
		sep := "?"
		if strings.Contains(*url, "?") {
			sep = "&"
		}
		return fmt.Sprintf("%s%sr=%d", *url, sep, seq.Add(1))
	}
	run := func(d time.Duration, record bool) (lat []time.Duration, errs, bad int64, bytes int64) {
		var mu sync.Mutex
		var wg sync.WaitGroup
		stop := time.Now().Add(d)
		for range *conns {
			wg.Go(func() {
				var mine []time.Duration
				for time.Now().Before(stop) {
					t := time.Now()
					res, err := client.Get(target())
					if err != nil {
						atomic.AddInt64(&errs, 1)
						continue
					}
					n, _ := io.Copy(io.Discard, res.Body)
					res.Body.Close()
					if res.StatusCode != http.StatusOK {
						atomic.AddInt64(&bad, 1)
					}
					atomic.AddInt64(&bytes, n)
					if record {
						mine = append(mine, time.Since(t))
					}
				}
				mu.Lock()
				lat = append(lat, mine...)
				mu.Unlock()
			})
		}
		wg.Wait()
		return
	}
	run(*warm, false)
	lat, errs, bad, bytes := run(*dur, true)
	slices.Sort(lat)
	pct := func(p float64) float64 {
		if len(lat) == 0 {
			return 0
		}
		return float64(lat[int(float64(len(lat)-1)*p)].Microseconds()) / 1000
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"url": *url, "unique": *unique, "conns": *conns, "requests": len(lat),
		"rps": int(float64(len(lat)) / dur.Seconds()), "p50_ms": pct(0.5), "p90_ms": pct(0.9), "p99_ms": pct(0.99),
		"errors": errs, "non_200": bad, "bytes_per_req": bytes / int64(max(1, len(lat))),
	})
}
