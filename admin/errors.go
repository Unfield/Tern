package admin

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Unfield/Tern/report"
)

// errorRow is a group of error reports as the admin screens show it.
type errorRow struct {
	ID        string    `admin:"key"`
	Message   string    `admin:"list,search"`
	Type      string    `admin:"list,search"`
	Source    string    `validate:"oneof=server|browser|job" admin:"list,filter"`
	Route     string    `admin:"list,search"`
	Count     int       `admin:"list"`
	LastSeen  time.Time `label:"Last seen" admin:"list"`
	FirstSeen time.Time `label:"First seen"`
	URL       string    `label:"Last URL"`
	RequestID string    `label:"Last request"`
	Stack     string    `admin:"textarea"`
}

// Errors is a resource for the error reports of r, grouped by cause, the
// most recent first: this process's, since the reporter keeps them in
// memory. Use report.Default() for the app's own reporter.
//
//	admin.New(admin.Options{Authorize: onlyAdmins}, admin.Errors(report.Default()))
func Errors(r *report.Reporter, opts ...Option) Res {
	return Resource("errors", errorStore{r}, append([]Option{Label("Errors", "Error"), ReadOnly()}, opts...)...)
}

type errorStore struct{ r *report.Reporter }

func (s errorStore) rows() []errorRow {
	out := []errorRow{}
	for _, g := range s.r.Groups() {
		var stack strings.Builder
		for _, f := range g.Last.Stack {
			mark := "  "
			if f.InApp {
				mark = "→ "
			}
			fmt.Fprintf(&stack, "%s%s\n    %s:%d\n", mark, f.Function, f.File, f.Line)
		}
		out = append(out, errorRow{
			ID: g.ID, Message: g.Message, Type: g.Type, Source: g.Source, Route: g.Route, Count: g.Count,
			LastSeen: g.LastSeen, FirstSeen: g.FirstSeen, URL: g.Last.URL, RequestID: g.Last.RequestID, Stack: stack.String(),
		})
	}
	return out
}

func (s errorStore) List(_ context.Context, q Query) ([]errorRow, int, error) {
	var out []errorRow
	text := strings.ToLower(q.Search)
	for _, e := range s.rows() {
		if text != "" && !strings.Contains(strings.ToLower(e.Message+" "+e.Type+" "+e.Route), text) {
			continue
		}
		if v, ok := q.Filters["source"]; ok && v != e.Source {
			continue
		}
		out = append(out, e)
	}
	switch q.Sort {
	case "count":
		slices.SortStableFunc(out, func(a, b errorRow) int { return cmp.Compare(a.Count, b.Count) })
	case "last_seen":
		slices.SortStableFunc(out, func(a, b errorRow) int { return a.LastSeen.Compare(b.LastSeen) })
	case "message", "type", "route", "source":
		slices.SortStableFunc(out, func(a, b errorRow) int {
			get := map[string]func(errorRow) string{"message": func(e errorRow) string { return e.Message }, "type": func(e errorRow) string { return e.Type }, "route": func(e errorRow) string { return e.Route }, "source": func(e errorRow) string { return e.Source }}[q.Sort]
			return strings.Compare(get(a), get(b))
		})
	}
	if q.Desc {
		slices.Reverse(out)
	}
	from := min(q.Offset, len(out))
	to := min(from+q.Limit, len(out))
	return out[from:to], len(out), nil
}

func (s errorStore) Get(_ context.Context, key string) (errorRow, error) {
	for _, e := range s.rows() {
		if e.ID == key {
			return e, nil
		}
	}
	return errorRow{}, ErrNotFound
}

func (errorStore) Create(context.Context, errorRow) (string, error)         { return "", errReadOnly }
func (errorStore) Update(context.Context, string, errorRow, []string) error { return errReadOnly }
func (errorStore) Delete(context.Context, string) error                     { return errReadOnly }

var errReadOnly = fmt.Errorf("admin: read only")
