package app

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Unfield/Tern/kit"

	"status/internal/monitor"
	"status/internal/store"
)

// MonitorInvalid is returned with a failed monitor form.
type MonitorInvalid struct {
	Values store.Monitor     `json:"values"`
	Errors map[string]string `json:"errors"`
}

// ReadMonitor parses and validates the monitor form, returning the errors
// to show next to its fields.
func ReadMonitor(ctx *kit.Ctx, id int64) (store.Monitor, map[string]string, error) {
	form, err := ctx.Form()
	if err != nil {
		return store.Monitor{}, nil, err
	}
	num := func(k string, def int) int {
		v, err := strconv.Atoi(strings.TrimSpace(form.Get(k)))
		if err != nil {
			return def
		}
		return v
	}
	m := store.Monitor{
		ID:              id,
		Name:            strings.TrimSpace(form.Get("name")),
		Type:            form.Get("type"),
		Target:          strings.TrimSpace(form.Get("target")),
		Method:          strings.ToUpper(strings.TrimSpace(form.Get("method"))),
		ExpectedStatus:  strings.TrimSpace(form.Get("expectedStatus")),
		Keyword:         form.Get("keyword"),
		IntervalSeconds: num("intervalSeconds", 60),
		TimeoutSeconds:  num("timeoutSeconds", 10),
		Retries:         num("retries", 1),
		CertExpiryDays:  num("certExpiryDays", 14),
		Group:           strings.TrimSpace(form.Get("group")),
		Public:          form.Get("public") == "on",
	}
	if m.Method == "" {
		m.Method = http.MethodGet
	}
	if m.ExpectedStatus == "" {
		m.ExpectedStatus = "200-399"
	}
	if m.Group == "" {
		m.Group = "Services"
	}
	return m, monitor.Validate(m), nil
}

// Groups returns the existing group names, for the form's suggestions.
func Groups(ctx *kit.Ctx) []string {
	monitors, err := Store.Monitors(ctx)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	for _, m := range monitors {
		if !seen[m.Group] {
			seen[m.Group] = true
			out = append(out, m.Group)
		}
	}
	return out
}
