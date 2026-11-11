// Package sentry is a report sink for Sentry, and for the services that
// take its protocol, such as GlitchTip and Bugsink:
//
//	report.New(report.Options{Sinks: []report.Sink{sentry.New(sentry.Options{})}})
//
// With no DSN it reads SENTRY_DSN. Reports keep their group as Sentry's
// fingerprint, so Sentry's issues match Tern's groups, and carry the trace
// they happened in.
package sentry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Unfield/Tern/internal/telemetry"
	"github.com/Unfield/Tern/report"
)

// Options configures the sink.
type Options struct {
	// DSN is the project's DSN: https://KEY@HOST/PROJECT. The default is
	// SENTRY_DSN's.
	DSN string
	// ServerName names the machine. The default is its host name.
	ServerName string
	// Client sends the requests. The default is http.DefaultClient.
	Client *http.Client
}

// Sink sends reports to Sentry. Make one with New.
type Sink struct {
	endpoint string
	auth     string
	server   string
	client   *http.Client
	err      error
}

// New returns a Sentry sink. A missing or malformed DSN makes every Send
// fail with the reason, once a minute in the reporter's errors.
func New(o Options) *Sink {
	dsn := o.DSN
	if dsn == "" {
		dsn = os.Getenv("SENTRY_DSN")
	}
	s := &Sink{server: o.ServerName, client: o.Client}
	if s.server == "" {
		s.server, _ = os.Hostname()
	}
	s.endpoint, s.auth, s.err = parseDSN(dsn)
	return s
}

// parseDSN reads https://KEY@HOST[/PATH]/PROJECT into the envelope
// endpoint and the auth header.
func parseDSN(dsn string) (endpoint, auth string, err error) {
	if dsn == "" {
		return "", "", errors.New("sentry: no DSN (set Options.DSN or SENTRY_DSN)")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil || u.Host == "" {
		return "", "", fmt.Errorf("sentry: malformed DSN")
	}
	key := u.User.Username()
	path := strings.TrimSuffix(u.Path, "/")
	i := strings.LastIndexByte(path, '/')
	if i < 0 || path[i+1:] == "" {
		return "", "", fmt.Errorf("sentry: the DSN has no project")
	}
	prefix, project := path[:i], path[i+1:]
	endpoint = u.Scheme + "://" + u.Host + prefix + "/api/" + project + "/envelope/"
	auth = "Sentry sentry_version=7, sentry_client=tern/1, sentry_key=" + key
	return endpoint, auth, nil
}

// Name names the sink in the reporter's errors.
func (*Sink) Name() string { return "sentry" }

// Send implements report.Sink: one envelope per report, as Sentry takes
// one event per envelope.
func (s *Sink) Send(ctx context.Context, reports []report.Report) error {
	if s.err != nil {
		return report.Permanent(s.err)
	}
	for _, r := range reports {
		body, err := envelope(r, s.server)
		if err != nil {
			return report.Permanent(err)
		}
		err = telemetry.Do(ctx, telemetry.Request{
			Client:      s.client,
			URL:         s.endpoint,
			ContentType: "application/x-sentry-envelope",
			Header:      http.Header{"X-Sentry-Auth": {s.auth}},
			Body:        body,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func envelope(r report.Report, server string) ([]byte, error) {
	ev := event(r, server)
	var b bytes.Buffer
	head, _ := json.Marshal(map[string]string{"event_id": r.ID, "sent_at": time.Now().UTC().Format(time.RFC3339)})
	b.Write(head)
	b.WriteString("\n{\"type\":\"event\"}\n")
	payload, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	b.Write(payload)
	b.WriteByte('\n')
	return b.Bytes(), nil
}

// event is the Sentry event for a report.
func event(r report.Report, server string) map[string]any {
	platform := "go"
	if r.Source == "browser" {
		platform = "javascript"
	}
	// Sentry lists frames oldest first.
	frames := make([]map[string]any, 0, len(r.Stack))
	for i := len(r.Stack) - 1; i >= 0; i-- {
		f := r.Stack[i]
		frames = append(frames, map[string]any{"function": f.Function, "filename": f.File, "abs_path": f.File, "lineno": f.Line, "in_app": f.InApp})
	}
	exc := map[string]any{"type": r.Type, "value": r.Message}
	if len(frames) > 0 {
		exc["stacktrace"] = map[string]any{"frames": frames}
	}
	if r.Panic {
		exc["mechanism"] = map[string]any{"type": "panic", "handled": true}
	}
	ev := map[string]any{
		"event_id":    r.ID,
		"timestamp":   r.Time.UTC().Format(time.RFC3339Nano),
		"platform":    platform,
		"level":       "error",
		"server_name": server,
		"exception":   map[string]any{"values": []any{exc}},
		"fingerprint": []string{r.Group},
		"tags":        map[string]string{"source": r.Source},
	}
	tags := ev["tags"].(map[string]string)
	if r.Release != "" {
		ev["release"] = r.Release
	}
	if r.Environment != "" {
		ev["environment"] = r.Environment
	}
	if r.Route != "" {
		ev["transaction"] = r.Route
		tags["route"] = r.Route
	}
	if r.RequestID != "" {
		tags["request_id"] = r.RequestID
	}
	if r.URL != "" || r.Method != "" {
		req := map[string]any{"url": r.URL, "method": r.Method}
		if r.UserAgent != "" {
			req["headers"] = map[string]string{"User-Agent": r.UserAgent}
		}
		ev["request"] = req
	}
	if r.User != "" {
		ev["user"] = map[string]string{"id": r.User}
	}
	if r.TraceID != "" {
		ev["contexts"] = map[string]any{"trace": map[string]string{"trace_id": r.TraceID, "span_id": r.SpanID}}
	}
	if len(r.Attrs) > 0 {
		ev["extra"] = r.Attrs
	}
	return ev
}
