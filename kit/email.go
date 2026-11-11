package kit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"slices"
	"strings"

	"github.com/Unfield/Tern/internal/htmltext"
	"github.com/Unfield/Tern/internal/ssr"
)

// Email is a rendered email template; see App.RenderEmail.
type Email struct {
	Subject string
	HTML    string
	// Text is the plain-text version, made from the HTML.
	Text string
}

// ErrNoEmails is returned by RenderEmail when the app has no src/emails.
var ErrNoEmails = ssr.ErrNoEmails

// RenderEmail renders the email template src/emails/<name>.svelte with
// props, which are passed to the component as JSON. The template's
// <svelte:head><title> is the subject:
//
//	<script lang="ts">
//		let { name, link }: { name: string; link: string } = $props();
//	</script>
//
//	<svelte:head><title>Welcome, {name}</title></svelte:head>
//	<h1>Welcome, {name}!</h1>
//	<p><a href={link}>Confirm your email</a></p>
//
// Templates render with the same engine as pages; under tern dev they
// update as you edit them, and /__tern/emails previews them. Send the
// result with the mail package.
func (a *App) RenderEmail(ctx context.Context, name string, props any) (Email, error) {
	r, err := a.emailRenderer()
	if err != nil {
		return Email{}, err
	}
	if props == nil {
		props = map[string]any{}
	}
	raw, err := json.Marshal(props)
	if err != nil {
		return Email{}, fmt.Errorf("kit: encoding the email's props: %w", err)
	}
	res, _, err := r.Email(ctx, name, raw)
	if err != nil {
		return Email{}, err
	}
	return Email{Subject: res.Subject, HTML: res.HTML, Text: htmltext.Convert(res.HTML)}, nil
}

func (a *App) emailRenderer() (ssr.EmailRenderer, error) {
	a.emailOnce.Do(func() {
		switch r := a.ssr.(type) {
		case *ssr.Dev:
			a.emails = r
			return
		case *ssr.Pool:
			a.emails = r
			return
		case ssr.EmailRenderer:
			a.emails = r // the native renderer: compiled emails, the engine when it must
			return
		case *ssr.Cache:
			if p, ok := r.Next().(ssr.EmailRenderer); ok {
				a.emails = p
				return
			}
		}
		// Server rendering is off for pages; emails still need it.
		if a.m.ServerBundle == "" {
			a.emailsErr = errors.New("kit: emails need the server bundle; build the app with tern build")
			return
		}
		pool, err := ssr.NewPool(a.m.ServerBundle, ssr.Options{Workers: 1, Logger: a.logger})
		if err != nil {
			a.emailsErr = err
			return
		}
		a.closers = append(a.closers, pool)
		a.emails = pool
	})
	return a.emails, a.emailsErr
}

// emailPreviews serves /__tern/emails under tern dev.
func (a *App) emailPreviews(w http.ResponseWriter, r *http.Request) {
	rend, err := a.emailRenderer()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/__tern/emails"), "/")
	_, raw, err := rend.Email(r.Context(), "", nil)
	if errors.Is(err, ssr.ErrNoEmails) {
		http.Error(w, "No email templates: add Svelte components to src/emails.", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var previews map[string]json.RawMessage
	_ = json.Unmarshal(raw, &previews)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if name == "" {
		names := make([]string, 0, len(previews))
		for n := range previews {
			names = append(names, n)
		}
		slices.Sort(names)
		var b strings.Builder
		b.WriteString(`<!doctype html><meta charset="utf-8"><title>Emails</title><body style="font:15px system-ui;margin:2rem"><h1>Emails</h1><p>Templates in src/emails, with the props from their <code>export const preview</code>.</p><ul>`)
		for _, n := range names {
			fmt.Fprintf(&b, `<li><a href="/__tern/emails/%[1]s">%[1]s</a> · <a href="/__tern/emails/%[1]s?text">text</a></li>`, html.EscapeString(n))
		}
		b.WriteString("</ul>")
		_, _ = w.Write([]byte(b.String()))
		return
	}
	props, ok := previews[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	e, err := a.RenderEmail(r.Context(), name, props)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.URL.Query().Has("text") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, "Subject: %s\n\n%s\n", e.Subject, e.Text)
		return
	}
	_, _ = w.Write([]byte(strings.Replace(e.HTML, "<body>", fmt.Sprintf(`<body><div style="font:13px system-ui;background:#fef3c7;padding:8px 12px;margin-bottom:16px">Subject: <b>%s</b> · <a href="/__tern/emails">all emails</a></div>`, html.EscapeString(e.Subject)), 1)))
}
