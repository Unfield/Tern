// Package notify sends monitor changes to notification channels: webhooks,
// Slack, Discord and email. Messages are sent from background jobs, so a
// failing channel is retried without slowing the checks down.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/mail"

	"status/internal/monitor"
	"status/internal/store"
)

// Types lists the channel types and the config fields each needs.
var Types = map[string][]string{
	"webhook": {"url"},
	"slack":   {"url"},
	"discord": {"url"},
	"email":   {"host", "port", "username", "password", "from", "to"},
}

// Validate checks a channel's settings.
func Validate(c store.Channel) map[string]string {
	errs := map[string]string{}
	if strings.TrimSpace(c.Name) == "" {
		errs["name"] = "Enter a name."
	}
	fields, ok := Types[c.Type]
	if !ok {
		errs["type"] = "Choose a channel type."
		return errs
	}
	get := func(k string) string { s, _ := c.Config[k].(string); return strings.TrimSpace(s) }
	switch c.Type {
	case "email":
		for _, f := range []string{"host", "port", "from", "to"} {
			if get(f) == "" {
				errs[f] = "Required."
			}
		}
		if p, err := strconv.Atoi(get("port")); get("port") != "" && (err != nil || p < 1 || p > 65535) {
			errs["port"] = "Enter a port number."
		}
	default:
		u, err := url.Parse(get(fields[0]))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			errs["url"] = "Enter the webhook URL."
		}
	}
	return errs
}

// Message is what a notification says.
func Message(c monitor.Change) (title, text string) {
	if c.Status == store.Down {
		return fmt.Sprintf("🔴 %s is down", c.Name), fmt.Sprintf("%s is down: %s (%s)", c.Name, c.Message, c.At.UTC().Format(time.RFC1123))
	}
	down := ""
	if c.Downtime > 0 {
		down = fmt.Sprintf(" after %s", c.Downtime.Round(time.Second))
	}
	return fmt.Sprintf("🟢 %s is back up", c.Name), fmt.Sprintf("%s is back up%s (%s)", c.Name, down, c.At.UTC().Format(time.RFC1123))
}

var client = &http.Client{Timeout: 15 * time.Second}

// Send delivers c to one channel.
func Send(ctx context.Context, ch store.Channel, c monitor.Change) error {
	title, text := Message(c)
	get := func(k string) string { s, _ := ch.Config[k].(string); return strings.TrimSpace(s) }
	switch ch.Type {
	case "webhook":
		return postJSON(ctx, get("url"), map[string]any{
			"event":   "monitor." + string(c.Status),
			"title":   title,
			"text":    text,
			"monitor": map[string]any{"id": c.MonitorID, "name": c.Name, "target": c.Target},
			"message": c.Message,
			"at":      c.At.UTC(),
		})
	case "slack":
		return postJSON(ctx, get("url"), map[string]string{"text": text})
	case "discord":
		return postJSON(ctx, get("url"), map[string]string{"content": text})
	case "email":
		return sendEmail(ctx, get, c)
	}
	return fmt.Errorf("unknown channel type %q", ch.Type)
}

func postJSON(ctx context.Context, target string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TernStatus/1.0")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		return fmt.Errorf("the webhook answered HTTP %d", res.StatusCode)
	}
	return nil
}

// Mailer renders and sends email notifications; main sets it, because
// rendering the src/emails template needs the app.
var Mailer func(ctx context.Context, smtp mail.SMTP, from string, to []string, c monitor.Change) error

func sendEmail(ctx context.Context, get func(string) string, c monitor.Change) error {
	port, _ := strconv.Atoi(get("port"))
	host := get("host")
	to := strings.FieldsFunc(get("to"), func(r rune) bool { return r == ',' || r == ' ' })
	if len(to) == 0 {
		return errors.New("no recipients")
	}
	if Mailer == nil {
		return errors.New("email is not set up")
	}
	// Local test servers such as Mailpit don't offer TLS.
	insecure := host == "localhost" || host == "127.0.0.1" || host == "::1"
	return Mailer(ctx, mail.SMTP{Host: host, Port: port, Username: get("username"), Password: get("password"), Insecure: insecure}, get("from"), to, c)
}
