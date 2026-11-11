package kit

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"github.com/Unfield/Tern/internal/jsonenc"
	"github.com/Unfield/Tern/internal/ssr"
	"github.com/Unfield/Tern/tracing"
)

// payload is the data sent to the client, both inline in the HTML shell
// and from __data.json and enhanced form actions.
type payload struct {
	// Type is "data", "error", "redirect", "success" or "failure".
	Type     string             `json:"type"`
	Route    string             `json:"route,omitempty"`
	Params   map[string]string  `json:"params,omitempty"`
	Status   int                `json:"status"`
	Nodes    []jsonenc.Verbatim `json:"nodes,omitempty"`
	Uses     []*nodeUses        `json:"uses,omitempty"`
	Reused   []int              `json:"reused,omitempty"`
	Failed   *int               `json:"failed,omitempty"`
	Error    *errorBody         `json:"error,omitempty"`
	Location string             `json:"location,omitempty"`
	Data     any                `json:"data,omitempty"`
	Form     any                `json:"form,omitempty"`
	Env      map[string]string  `json:"env,omitempty"`
	Locale   string             `json:"locale,omitempty"`
	HasForm  bool               `json:"hasForm,omitempty"`
	// Deferred says the nodes have deferred values' placeholders, so the
	// renderers look for them only then.
	Deferred bool `json:"deferred,omitempty"`
	// Errors asks the client to send its uncaught errors to
	// BrowserErrorsPath, when the App has a reporter.
	Errors bool `json:"errors,omitempty"`
	// values are the loads' results that Nodes encodes, when there's one
	// for each node: native rendering reads them instead of the JSON.
	values []any
}

// renderPayload is the payload with the loads' values in place of their
// JSON, for native rendering: it makes the same JavaScript values.
type renderPayload struct {
	payload
	Nodes []any `json:"nodes,omitempty"`
}

// asValue is p for the renderer to read without parsing its JSON, or nil
// when a node's value isn't there.
func (p payload) asValue() any {
	if len(p.values) != len(p.Nodes) {
		return nil
	}
	return renderPayload{payload: p, Nodes: p.values}
}

type errorBody struct {
	Message string `json:"message"`
}

func (a *App) writeDataJSON(c *Ctx, status int, p payload) {
	p.Locale = c.rootCtx().locale
	p.Deferred = len(c.pendingDeferreds()) > 0
	b, err := jsonenc.Marshal(p)
	if err != nil {
		a.logError(c, fmt.Errorf("kit: encoding response: %w", err))
		b = []byte(`{"type":"error","status":500,"error":{"message":"Internal Error"}}`)
		status = http.StatusInternalServerError
	}
	c.applyHeaders()
	h := c.writer.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "private, no-store")
	c.writer.WriteHeader(status)
	_, _ = c.writer.Write(b)
	if p.Type == "data" && len(c.pendingDeferreds()) > 0 {
		// Deferred values follow as one JSON object per line.
		_, _ = c.writer.Write([]byte("\n"))
		_ = http.NewResponseController(c.writer).Flush()
		a.streamDeferred(c, func(chunk []byte) {
			_, _ = c.writer.Write(append(chunk, '\n'))
		})
	}
}

// shell is a parsed src/app.html.
type shell struct {
	parts []string // text between placeholders
	slots []string // placeholder names, len(parts)-1
}

var placeholders = []string{"%tern.head%", "%tern.body%", "%tern.assets%", "%tern.nonce%", "%tern.lang%"}

func parseShell(src string) (*shell, error) {
	if src == "" {
		return nil, errors.New("kit: src/app.html is empty")
	}
	for _, required := range []string{"%tern.head%", "%tern.body%"} {
		if !strings.Contains(src, required) {
			return nil, fmt.Errorf("kit: src/app.html must contain %s", required)
		}
	}
	s := &shell{}
	rest := src
	for {
		idx, which := -1, ""
		for _, p := range placeholders {
			if i := strings.Index(rest, p); i >= 0 && (idx < 0 || i < idx) {
				idx, which = i, p
			}
		}
		if idx < 0 {
			s.parts = append(s.parts, rest)
			return s, nil
		}
		s.parts = append(s.parts, rest[:idx])
		s.slots = append(s.slots, which)
		rest = rest[idx+len(which):]
	}
}

// appendTo writes the document into b: the shell with head's and body's
// pieces in their places, in one buffer of the right size.
func (s *shell) appendTo(b []byte, head, body []string, nonce, base, lang string) []byte {
	n := 0
	for _, p := range s.parts {
		n += len(p)
	}
	size := func(pieces []string) (m int) {
		for _, p := range pieces {
			m += len(p)
		}
		return m
	}
	for _, slot := range s.slots {
		switch slot {
		case "%tern.head%":
			n += size(head)
		case "%tern.body%":
			n += size(body)
		case "%tern.nonce%":
			n += len(nonce)
		case "%tern.assets%":
			n += len(base)
		case "%tern.lang%":
			n += len(lang)
		}
	}
	b = slices.Grow(b[:0], n)
	for i, part := range s.parts {
		b = append(b, part...)
		if i < len(s.slots) {
			switch s.slots[i] {
			case "%tern.head%":
				for _, p := range head {
					b = append(b, p...)
				}
			case "%tern.body%":
				for _, p := range body {
					b = append(b, p...)
				}
			case "%tern.nonce%":
				b = append(b, nonce...)
			case "%tern.assets%":
				b = append(b, base...)
			case "%tern.lang%":
				b = append(b, lang...)
			}
		}
	}
	return b
}

// render is the document as a string.
func (s *shell) render(head, body, nonce, base, lang string) string {
	return string(s.appendTo(nil, []string{head}, []string{body}, nonce, base, lang))
}

// docBuffers hold page payloads and documents between requests: a page
// is written into one, sent, and the buffer used again.
var docBuffers = sync.Pool{New: func() any { return new([]byte) }}

func getBuffer() *[]byte { return docBuffers.Get().(*[]byte) }

func putBuffer(b *[]byte) {
	if cap(*b) > 4<<20 {
		return // a rare giant page: let it go
	}
	*b = (*b)[:0]
	docBuffers.Put(b)
}

// writeDocument responds with the HTML shell for a page or error.
func (a *App) writeDocument(c *Ctx, status int, def *RouteDef, p payload) {
	if len(a.publicEnv) > 0 {
		p.Env = a.publicEnv // full page loads only; the client keeps it
	}
	p.Locale = c.rootCtx().locale
	p.Deferred = len(c.pendingDeferreds()) > 0
	p.Errors = a.reporter() != nil
	dataBuf := getBuffer()
	defer putBuffer(dataBuf)
	data, err := jsonenc.Append((*dataBuf)[:0], p) // escapes <, > and & for safe inlining
	if err != nil {
		a.logError(c, fmt.Errorf("kit: encoding page data: %w", err))
		status = http.StatusInternalServerError
		data = []byte(`{"type":"error","status":500,"error":{"message":"Internal Error"}}`)
	} else {
		*dataBuf = data
	}

	nonce := newNonce()
	var head strings.Builder
	shell := a.shell
	if a.dev != nil {
		var err error
		if shell, err = a.dev.shell(); err != nil {
			a.logError(c, err)
			http.Error(c.writer, err.Error(), http.StatusInternalServerError)
			return
		}
		sctx, span := tracing.Start(c.Request.Context(), "dev: collect styles")
		css := a.dev.styles(sctx, a.components(def))
		span.End()
		if css != "" {
			fmt.Fprintf(&head, "\n\t\t<style data-tern-dev nonce=\"%s\">%s</style>", nonce, css)
		}
		for _, src := range a.dev.scripts(a.m.BasePath) {
			fmt.Fprintf(&head, "\n\t\t<script type=\"module\" src=\"%s\" nonce=\"%s\"></script>", html.EscapeString(src), nonce)
		}
	} else if a.assets != nil {
		components := a.components(def)
		if l := c.rootCtx().locale; l != "" {
			// Preload the locale's messages with the page's code.
			components = append(components, "src/locales/"+l+".json")
			if d := a.i18n.Default; d != l {
				components = append(components, "src/locales/"+d+".json")
			}
		}
		scripts, styles := a.assets.forPage(components)
		base := a.m.BasePath
		for _, href := range styles {
			fmt.Fprintf(&head, "\n\t\t<link rel=\"stylesheet\" href=\"%s\">", html.EscapeString(base+href))
		}
		for _, href := range scripts[1:] {
			fmt.Fprintf(&head, "\n\t\t<link rel=\"modulepreload\" href=\"%s\">", html.EscapeString(base+href))
		}
		fmt.Fprintf(&head, "\n\t\t<script type=\"module\" src=\"%s\" nonce=\"%s\"></script>", html.EscapeString(base+scripts[0]), nonce)
	}
	for _, fn := range a.head {
		if h := fn(c, nonce); h != "" {
			head.WriteString("\n\t\t")
			head.WriteString(h)
		}
	}
	streaming := len(c.pendingDeferreds()) > 0 && c.Request.Method != http.MethodHead
	if streaming {
		fmt.Fprintf(&head, "\n\t\t<script nonce=\"%s\">%s</script>", nonce, resolveScript)
	}
	// The head: the rendered head first, then the assets and the data.
	headParts := []string{head.String(), "\n\t\t<script type=\"application/json\" id=\"__tern_data\" nonce=\"", nonce, "\">", unsafeString(data), "</script>"}
	bodyParts := []string{`<div id="tern-root" style="display: contents"></div>`}
	if a.ssr != nil && (def == nil || def.Page == nil || !def.Page.NoSSR) {
		ctx := context.WithValue(c.Request.Context(), liveLocalsKey{}, maps.Clone(c.Locals))
		if v := p.asValue(); v != nil {
			ctx = ssr.WithPayloadValue(ctx, v)
		}
		ctx, span := renderSpan(ctx)
		res, err := a.ssr.Render(ctx, data, absoluteURL(c.Request, a.m.BasePath))
		span.RecordError(err)
		span.End()
		switch {
		case err != nil:
			// Render on the client instead; the page still works.
			a.logger.WarnContext(c.Request.Context(), "tern: server rendering failed; falling back to client rendering",
				"path", c.Request.URL.Path, "route", c.routeID, "error", err)
		case !res.Skip:
			if res.Head != "" {
				headParts = append([]string{"\n\t\t", res.Head}, headParts...)
			}
			bodyParts = []string{`<div id="tern-root" data-tern-ssr style="display: contents">`, res.Body, `</div>`}
		}
	}
	docBuf := getBuffer()
	defer putBuffer(docBuf)
	doc := shell.appendTo(*docBuf, headParts, bodyParts, nonce, a.m.BasePath, a.htmlLang(c))
	*docBuf = doc

	c.applyHeaders()
	h := c.writer.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	if a.opts.csp != "" {
		name := "Content-Security-Policy"
		if a.opts.cspReportOnly {
			name += "-Report-Only"
		}
		h.Set(name, withNonce(a.opts.csp, nonce))
	}
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "private, no-cache")
	}
	c.writer.WriteHeader(status)
	if c.Request.Method == http.MethodHead {
		return
	}
	if !streaming {
		_, _ = c.writer.Write(doc)
		return
	}
	// Send the page now, then each deferred value as it resolves.
	before, after := splitBody(doc)
	_, _ = c.writer.Write(before)
	_ = http.NewResponseController(c.writer).Flush()
	a.streamDeferred(c, func(chunk []byte) {
		fmt.Fprintf(c.writer, "<script nonce=\"%s\">__tern_resolve(%s)</script>\n", nonce, chunk)
	})
	_, _ = c.writer.Write(after)
}

// unsafeString is b as a string without copying, for b that doesn't change
// while the string is used.
func unsafeString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// absoluteURL reconstructs the URL the client requested.
func absoluteURL(r *http.Request, base string) string {
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host + base + r.URL.RequestURI()
}

// components returns the Svelte files a page renders, for preloading.
func (a *App) components(def *RouteDef) []string {
	layouts := a.m.Fallback.Layouts
	if def != nil {
		layouts = def.Layouts
	}
	var out []string
	for _, li := range layouts {
		if c := a.m.Layouts[li].Component; c != "" {
			out = append(out, c)
		}
	}
	if def != nil && def.Page != nil {
		out = append(out, def.Page.Component)
	}
	return out
}

// withNonce adds 'nonce-…' to a policy's script-src directive, creating it
// from default-src (or 'self') when absent.
func withNonce(policy, nonce string) string {
	token := "'nonce-" + nonce + "'"
	directives := strings.Split(policy, ";")
	var defaultSrc string
	for i, d := range directives {
		fields := strings.Fields(d)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "script-src":
			directives[i] = strings.TrimSpace(d) + " " + token
			return strings.Join(trimAll(directives), "; ")
		case "default-src":
			defaultSrc = strings.Join(fields[1:], " ")
		}
	}
	if defaultSrc == "" {
		defaultSrc = "'self'"
	}
	return strings.Join(append(trimAll(directives), "script-src "+defaultSrc+" "+token), "; ")
}

func trimAll(parts []string) []string {
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func newNonce() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.RawStdEncoding.EncodeToString(b[:])
}
