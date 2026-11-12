// Package mcp serves an app's tools to AI assistants over the Model Context
// Protocol, at /mcp, with Streamable HTTP. Tools are Go functions with
// typed arguments and results; with plugins/oidc, users authorize each
// assistant on the app's own consent screen, and tools run as that user.
//
//	server := mcp.New(mcp.Options{
//		Name:  "Shelf",
//		Tools: []mcp.Tool{mcp.NewTool("list_links", "List the user's links.", listLinks, mcp.ReadOnly())},
//		OAuth: provider,
//		SignIn: func(ctx *kit.Ctx, t *oidc.AccessToken) error { ... },
//	})
//	app, err := ternApp(kit.WithPlugins(acc, provider, server))
//
// It serves protocol version 2026-07-28, which is stateless, and 2025-11-25,
// 2025-06-18 and 2025-03-26, which start with initialize, on the same
// endpoint, without sessions. See docs/mcp-server.md and ADR 0023.
package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/plugins/oidc"
)

// Modern is the stateless protocol version: every request carries its
// version, and there's no initialize.
const Modern = "2026-07-28"

// Versions are the protocol versions served, newest first.
var Versions = []string{Modern, "2025-11-25", "2025-06-18", "2025-03-26"}

// legacy are the versions that start with initialize.
var legacy = Versions[1:]

// Options configure the server.
type Options struct {
	// Path is the endpoint; the default is "/mcp".
	Path string
	// Name and Version identify the server to clients; the default name is
	// "tern". Title is a display name, such as "Shelf".
	Name, Version, Title string
	// Instructions tell the assistant's model how to use the tools, such
	// as "Links are a user's reading list. Save one when the user asks to
	// read something later."
	Instructions string
	Tools        []Tool

	// OAuth is the app's OpenID Connect provider. Requests then need an
	// access token issued for this server (its Resource), and SignIn makes
	// the token's user the request's.
	OAuth *oidc.Plugin
	// Resource is the server's canonical URL, which tokens must be issued
	// for. The default is the provider's issuer origin and Path, such as
	// "https://example.com/mcp".
	Resource string
	// Scopes every request needs, such as "mcp". Tools can need more.
	Scopes []string
	// SignIn makes the token's subject the signed-in user of the request,
	// such as with accounts' ActAs. An error refuses the request with 401.
	SignIn func(ctx *kit.Ctx, token *oidc.AccessToken) error

	// Authorize checks requests without OAuth, such as an API key in a
	// header. An error from kit.Error refuses the request with its status.
	Authorize func(ctx *kit.Ctx) error

	// AllowedOrigins are browser origins that may call the server, such as
	// "https://inspector.example.com". Requests from browsers on other
	// origins are refused (DNS rebinding protection); clients that aren't
	// browsers send no Origin.
	AllowedOrigins []string
}

// Server is the MCP server, a kit.Plugin.
type Server struct {
	opts     Options
	tools    []Tool
	byName   map[string]*Tool
	metadata string // the protected resource metadata URL
}

// New returns the server.
func New(opts Options) *Server {
	if opts.Path == "" {
		opts.Path = "/mcp"
	}
	opts.Path = "/" + strings.Trim(opts.Path, "/")
	if opts.Name == "" {
		opts.Name = "tern"
	}
	if opts.Version == "" {
		opts.Version = "1.0.0"
	}
	s := &Server{opts: opts, tools: opts.Tools, byName: map[string]*Tool{}}
	for i := range s.tools {
		s.byName[s.tools[i].name] = &s.tools[i]
	}
	return s
}

// Name implements kit.Plugin.
func (*Server) Name() string { return "mcp" }

// Setup implements kit.Plugin.
func (s *Server) Setup(st *kit.Setup) error {
	seen := map[string]bool{}
	for _, t := range s.tools {
		if t.err != nil {
			return t.err
		}
		if seen[t.name] {
			return fmt.Errorf("mcp: two tools are named %s", t.name)
		}
		seen[t.name] = true
	}
	if s.opts.OAuth != nil {
		if s.opts.SignIn == nil {
			return errors.New("mcp: Options.SignIn is required with OAuth")
		}
		if s.opts.Resource == "" {
			u, err := url.Parse(s.opts.OAuth.Issuer())
			if err != nil {
				return err
			}
			s.opts.Resource = u.Scheme + "://" + u.Host + s.opts.Path
		}
		r, err := url.Parse(s.opts.Resource)
		if err != nil || r.Host == "" {
			return fmt.Errorf("mcp: invalid Resource %q", s.opts.Resource)
		}
		// Protected resource metadata (RFC 9728), at the path clients try
		// first and at the root.
		s.metadata = r.Scheme + "://" + r.Host + "/.well-known/oauth-protected-resource" + r.Path
		st.Handle("GET /.well-known/oauth-protected-resource"+r.Path, http.HandlerFunc(s.resourceMetadata))
		st.Handle("GET /.well-known/oauth-protected-resource", http.HandlerFunc(s.resourceMetadata))
	}
	st.Endpoint(s.opts.Path, s.serve)
	// Requests carry tokens, not cookies; the Origin check is below.
	st.AllowCrossOrigin(s.opts.Path)
	return nil
}

func (s *Server) resourceMetadata(w http.ResponseWriter, _ *http.Request) {
	doc := map[string]any{
		"resource":                 s.opts.Resource,
		"authorization_servers":    []string{s.opts.OAuth.Issuer()},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            cmpOr(s.opts.Title, s.opts.Name),
	}
	if scopes := s.allScopes(); len(scopes) > 0 {
		doc["scopes_supported"] = scopes
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(doc)
}

func (s *Server) allScopes() []string {
	out := slices.Clone(s.opts.Scopes)
	for _, t := range s.tools {
		for _, sc := range t.scopes {
			if !slices.Contains(out, sc) {
				out = append(out, sc)
			}
		}
	}
	return out
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// rpcRequest is a JSON-RPC request or notification.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeHeaderMismatch = -32020
	codeUnsupported    = -32022
)

const maxBody = 1 << 20

func (s *Server) serve(c *kit.Ctx) error {
	w, r := c.Writer(), c.Request
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	if origin := r.Header.Get("Origin"); origin != "" {
		if !s.originAllowed(r, origin) {
			s.write(w, http.StatusForbidden, nil, nil, &rpcError{Code: codeInvalidRequest, Message: "origin not allowed"})
			return nil
		}
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Expose-Headers", "WWW-Authenticate")
		h.Add("Vary", "Origin")
		if r.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Methods", "POST")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version, Mcp-Method, Mcp-Name")
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
	}
	if r.Method != http.MethodPost {
		// No GET stream: 2026-07-28 removed it, and earlier versions let a
		// server decline it.
		h.Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil
	}
	if ok, err := s.authorize(c, s.opts.Scopes); !ok {
		return err
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		s.write(w, http.StatusRequestEntityTooLarge, nil, nil, &rpcError{Code: codeInvalidRequest, Message: "the request is too large"})
		return nil //nolint:nilerr // answered above
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		msg := "parse error"
		if trimmed := strings.TrimSpace(string(body)); strings.HasPrefix(trimmed, "[") {
			msg = "batches aren't supported; send one request per POST"
		}
		s.write(w, http.StatusBadRequest, nil, nil, &rpcError{Code: codeParse, Message: msg})
		return nil
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		s.write(w, http.StatusBadRequest, req.ID, nil, &rpcError{Code: codeInvalidRequest, Message: "not a JSON-RPC 2.0 request"})
		return nil
	}

	version, rerr, status := s.version(r, &req)
	if rerr != nil {
		s.write(w, status, req.ID, nil, rerr)
		return nil
	}
	if len(req.ID) == 0 || string(req.ID) == "null" {
		w.WriteHeader(http.StatusAccepted) // a notification
		return nil
	}
	result, rerr, status := s.handle(c, version, &req)
	if rerr != nil {
		s.write(w, status, req.ID, nil, rerr)
		return nil
	}
	if result == nil {
		return nil // already answered, such as with 403 for a scope
	}
	if version == Modern {
		result["resultType"] = "complete"
		meta, _ := result["_meta"].(map[string]any)
		if meta == nil {
			meta = map[string]any{}
		}
		meta["io.modelcontextprotocol/serverInfo"] = s.serverInfo()
		result["_meta"] = meta
	}
	s.write(w, http.StatusOK, req.ID, result, nil)
	return nil
}

func (s *Server) write(w http.ResponseWriter, status int, id json.RawMessage, result any, rerr *rpcError) {
	res := map[string]any{"jsonrpc": "2.0"}
	if len(id) > 0 {
		res["id"] = id
	}
	if rerr != nil {
		res["error"] = rerr
	} else {
		res["result"] = result
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) serverInfo() map[string]any {
	info := map[string]any{"name": s.opts.Name, "version": s.opts.Version}
	if s.opts.Title != "" {
		info["title"] = s.opts.Title
	}
	return info
}

// originAllowed accepts the server's own origin and AllowedOrigins.
func (s *Server) originAllowed(r *http.Request, origin string) bool {
	if slices.Contains(s.opts.AllowedOrigins, origin) {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

// version finds the request's protocol version and checks the headers
// 2026-07-28 requires against the body.
func (s *Server) version(r *http.Request, req *rpcRequest) (string, *rpcError, int) {
	var p struct {
		Meta struct {
			Version string `json:"io.modelcontextprotocol/protocolVersion"`
		} `json:"_meta"`
		Name string `json:"name"`
		URI  string `json:"uri"`
	}
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &p)
	}
	header := r.Header.Get("MCP-Protocol-Version")
	if req.Method == "initialize" {
		return legacy[0], nil, 0 // answered with the version it negotiates
	}
	switch {
	case p.Meta.Version != "":
		if !slices.Contains(Versions, p.Meta.Version) {
			return "", unsupported(p.Meta.Version), http.StatusBadRequest
		}
		if header != p.Meta.Version {
			return "", &rpcError{Code: codeHeaderMismatch, Message: fmt.Sprintf("Header mismatch: MCP-Protocol-Version %q does not match the body's %q", header, p.Meta.Version)}, http.StatusBadRequest
		}
		if p.Meta.Version == Modern {
			if m := decodeHeader(r.Header.Get("Mcp-Method")); m != req.Method {
				return "", &rpcError{Code: codeHeaderMismatch, Message: fmt.Sprintf("Header mismatch: Mcp-Method %q does not match the body's %q", m, req.Method)}, http.StatusBadRequest
			}
			if req.Method == "tools/call" || req.Method == "resources/read" || req.Method == "prompts/get" {
				want := p.Name
				if req.Method == "resources/read" {
					want = p.URI
				}
				if n := decodeHeader(r.Header.Get("Mcp-Name")); n != want {
					return "", &rpcError{Code: codeHeaderMismatch, Message: fmt.Sprintf("Header mismatch: Mcp-Name %q does not match the body's %q", n, want)}, http.StatusBadRequest
				}
			}
		}
		return p.Meta.Version, nil, 0
	case header == "":
		// Before 2025-06-18, clients sent no header after initialize.
		return "2025-03-26", nil, 0
	case header == Modern:
		return "", &rpcError{Code: codeHeaderMismatch, Message: "Header mismatch: the body has no io.modelcontextprotocol/protocolVersion"}, http.StatusBadRequest
	case slices.Contains(legacy, header):
		return header, nil, 0
	default:
		return "", unsupported(header), http.StatusBadRequest
	}
}

func unsupported(v string) *rpcError {
	return &rpcError{Code: codeUnsupported, Message: "Unsupported protocol version", Data: map[string]any{"supported": Versions, "requested": v}}
}

// decodeHeader decodes the =?base64?…?= form of header values that aren't
// plain ASCII.
func decodeHeader(v string) string {
	if inner, ok := strings.CutPrefix(v, "=?base64?"); ok {
		if inner, ok = strings.CutSuffix(inner, "?="); ok {
			if b, err := decodeBase64(inner); err == nil {
				return string(b)
			}
		}
	}
	return v
}
