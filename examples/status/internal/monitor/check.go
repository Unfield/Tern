// Package monitor checks services and tracks whether they are up.
package monitor

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"status/internal/store"
)

// Result is the outcome of one check.
type Result struct {
	OK          bool
	Latency     time.Duration
	Message     string
	CertExpires *time.Time
}

// Types lists the kinds of monitor, for forms.
var Types = []string{"http", "tcp", "dns", "tls"}

// Validate checks a monitor's settings before it is saved.
func Validate(m store.Monitor) map[string]string {
	errs := map[string]string{}
	if strings.TrimSpace(m.Name) == "" {
		errs["name"] = "Enter a name."
	}
	switch m.Type {
	case "http":
		u, err := url.Parse(m.Target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs["target"] = "Enter an http:// or https:// URL."
		}
		if _, err := parseStatuses(m.ExpectedStatus); err != nil {
			errs["expectedStatus"] = err.Error()
		}
	case "tcp":
		if _, port, err := net.SplitHostPort(m.Target); err != nil || port == "" {
			errs["target"] = "Enter a host and port, such as db.internal:5432."
		}
	case "dns", "tls":
		if m.Target == "" || strings.ContainsAny(m.Target, "/ ") {
			errs["target"] = "Enter a host name, such as example.com."
		}
	default:
		errs["type"] = "Choose a monitor type."
	}
	if m.IntervalSeconds < 5 || m.IntervalSeconds > 86400 {
		errs["intervalSeconds"] = "Check between every 5 seconds and once a day."
	}
	if m.TimeoutSeconds < 1 || m.TimeoutSeconds > 120 {
		errs["timeoutSeconds"] = "Use a timeout between 1 and 120 seconds."
	}
	if m.Retries < 0 || m.Retries > 10 {
		errs["retries"] = "Use between 0 and 10 retries."
	}
	return errs
}

// Check runs one check of m.
func Check(ctx context.Context, m store.Monitor) Result {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(m.TimeoutSeconds)*time.Second)
	defer cancel()
	start := time.Now()
	var r Result
	switch m.Type {
	case "http":
		r = checkHTTP(ctx, m)
	case "tcp":
		r = checkTCP(ctx, m.Target)
	case "dns":
		r = checkDNS(ctx, m.Target)
	case "tls":
		r = checkTLS(ctx, m.Target, m.CertExpiryDays)
	default:
		r = Result{Message: "unknown monitor type " + m.Type}
	}
	r.Latency = time.Since(start)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) && !r.OK {
		r.Message = fmt.Sprintf("timed out after %ds", m.TimeoutSeconds)
	}
	return r
}

var client = &http.Client{
	Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("more than 10 redirects")
		}
		return nil
	},
}

func checkHTTP(ctx context.Context, m store.Monitor) Result {
	method := m.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, m.Target, nil)
	if err != nil {
		return Result{Message: err.Error()}
	}
	req.Header.Set("User-Agent", "TernStatus/1.0 (+https://github.com/Unfield/Tern)")
	res, err := client.Do(req)
	if err != nil {
		return Result{Message: shortError(err)}
	}
	defer func() { _ = res.Body.Close() }()
	var r Result
	if res.TLS != nil && len(res.TLS.PeerCertificates) > 0 {
		exp := res.TLS.PeerCertificates[0].NotAfter
		r.CertExpires = &exp
	}
	ok, _ := parseStatuses(m.ExpectedStatus)
	if !ok(res.StatusCode) {
		r.Message = fmt.Sprintf("HTTP %d, expected %s", res.StatusCode, m.ExpectedStatus)
		return r
	}
	if m.Keyword != "" {
		body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		if err != nil {
			r.Message = shortError(err)
			return r
		}
		if !strings.Contains(string(body), m.Keyword) {
			r.Message = fmt.Sprintf("HTTP %d, but %q is not in the response", res.StatusCode, m.Keyword)
			return r
		}
	}
	r.OK = true
	r.Message = fmt.Sprintf("HTTP %d", res.StatusCode)
	return r
}

// parseStatuses parses "200-299,301" into a matcher.
func parseStatuses(spec string) (func(int) bool, error) {
	if strings.TrimSpace(spec) == "" {
		spec = "200-399"
	}
	type rng struct{ lo, hi int }
	var ranges []rng
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		lo, hi, isRange := strings.Cut(part, "-")
		a, err1 := strconv.Atoi(strings.TrimSpace(lo))
		b := a
		var err2 error
		if isRange {
			b, err2 = strconv.Atoi(strings.TrimSpace(hi))
		}
		if err1 != nil || err2 != nil || a < 100 || b > 599 || a > b {
			return nil, fmt.Errorf("use status codes and ranges such as 200-299,301")
		}
		ranges = append(ranges, rng{a, b})
	}
	return func(code int) bool {
		for _, r := range ranges {
			if code >= r.lo && code <= r.hi {
				return true
			}
		}
		return false
	}, nil
}

func checkTCP(ctx context.Context, target string) Result {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return Result{Message: shortError(err)}
	}
	_ = conn.Close()
	return Result{OK: true, Message: "port open"}
}

func checkDNS(ctx context.Context, host string) Result {
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return Result{Message: shortError(err)}
	}
	if len(addrs) > 3 {
		addrs = append(addrs[:3], fmt.Sprintf("and %d more", len(addrs)-3))
	}
	return Result{OK: true, Message: strings.Join(addrs, ", ")}
}

func checkTLS(ctx context.Context, host string, warnDays int) Result {
	addr := host
	if _, _, err := net.SplitHostPort(host); err != nil {
		addr = net.JoinHostPort(host, "443")
	}
	d := tls.Dialer{Config: &tls.Config{MinVersion: tls.VersionTLS12}}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return Result{Message: shortError(err)}
	}
	defer func() { _ = conn.Close() }()
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Result{Message: "no certificate"}
	}
	exp := certs[0].NotAfter
	days := int(time.Until(exp).Hours() / 24)
	r := Result{CertExpires: &exp, Message: fmt.Sprintf("certificate valid for %d more days", days)}
	if days < warnDays {
		r.Message = fmt.Sprintf("certificate expires in %d days", days)
		return r
	}
	r.OK = true
	return r
}

// shortError keeps the useful end of network errors, such as
// "connection refused" instead of "dial tcp 127.0.0.1:9: connect: …".
func shortError(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return "no such host"
		}
		return "DNS: " + dnsErr.Err
	}
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 && !strings.Contains(msg, "x509") {
		msg = msg[i+2:]
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timed out"
	}
	return msg
}
