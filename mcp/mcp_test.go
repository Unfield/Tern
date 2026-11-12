package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/mcp"
	"github.com/Unfield/Tern/plugins/oidc"
	"github.com/Unfield/Tern/plugins/sessions"
)

type NewLink struct {
	URL   string   `json:"url" validate:"required,url,max=2000" description:"The page's address"`
	Title string   `json:"title,omitempty" validate:"max=200"`
	Tags  []string `json:"tags,omitempty" validate:"max=5,oneof=go|svelte|later"`
}

type Link struct {
	ID    int64  `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	Note  string `json:"note,omitempty"`
}

type Who struct{}

func tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("save_link", "Save a link to read later.", func(_ *kit.Ctx, in NewLink) (Link, error) {
			if strings.Contains(in.URL, "taken") {
				return Link{}, kit.Error(409, "That link is already saved.")
			}
			if strings.Contains(in.URL, "boom") {
				return Link{}, errors.New("database password is hunter2")
			}
			if strings.Contains(in.URL, "panic") {
				panic("oh no")
			}
			return Link{ID: 1, URL: in.URL, Title: in.Title}, nil
		}, mcp.Title("Save a link"), mcp.Additive()),
		mcp.NewTool("whoami", "Who the token belongs to.", func(c *kit.Ctx, _ Who) (string, error) {
			who, _ := c.Locals["user"].(string)
			if who == "" {
				return "", kit.Redirect(303, "/login")
			}
			return "you are " + who, nil
		}, mcp.ReadOnly()),
		mcp.NewTool("delete_all", "Delete every link.", func(*kit.Ctx, Who) (string, error) { return "deleted", nil },
			mcp.Destructive(), mcp.Scopes("links:delete")),
	}
}

func newApp(t *testing.T, opts mcp.Options) *httptest.Server {
	t.Helper()
	opts.Tools = tools()
	opts.Name, opts.Title, opts.Instructions = "shelf", "Shelf", "Links are a reading list."
	app, err := kit.New(&kit.Manifest{AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`},
		kit.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		kit.WithPlugins(mcp.New(opts)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app)
	t.Cleanup(srv.Close)
	return srv
}

type reply struct {
	Status int
	Header http.Header
	Result map[string]any
	Error  struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
}

// post sends a JSON-RPC message with the headers in h.
func post(t *testing.T, u string, body any, h map[string]string) reply {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, u, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range h {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out struct {
		Result map[string]any `json:"result"`
		Error  json.RawMessage
	}
	raw, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(raw, &out)
	r := reply{Status: res.StatusCode, Header: res.Header, Result: out.Result}
	_ = json.Unmarshal(out.Error, &r.Error)
	return r
}

// modern sends a 2026-07-28 request, with its headers.
func modern(t *testing.T, u, method string, params map[string]any, h map[string]string) reply {
	t.Helper()
	if params == nil {
		params = map[string]any{}
	}
	params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": mcp.Modern, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
	headers := map[string]string{"MCP-Protocol-Version": mcp.Modern, "Mcp-Method": method}
	if name, ok := params["name"].(string); ok {
		headers["Mcp-Name"] = name
	}
	for k, v := range h {
		headers[k] = v
	}
	return post(t, u, map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}, headers)
}

func text(r reply) string {
	content, _ := r.Result["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	return content[0].(map[string]any)["text"].(string)
}

func TestModernProtocol(t *testing.T) {
	u := newApp(t, mcp.Options{}).URL + "/mcp"

	r := modern(t, u, "server/discover", nil, nil)
	if r.Status != 200 || r.Result["resultType"] != "complete" || r.Result["instructions"] != "Links are a reading list." || r.Result["cacheScope"] != "public" {
		t.Fatalf("discover: %d %v", r.Status, r.Result)
	}
	if info := r.Result["_meta"].(map[string]any)["io.modelcontextprotocol/serverInfo"].(map[string]any); info["name"] != "shelf" || info["title"] != "Shelf" {
		t.Errorf("serverInfo %v", info)
	}

	r = modern(t, u, "tools/list", nil, nil)
	list := r.Result["tools"].([]any)
	if len(list) != 3 || r.Result["ttlMs"] == nil {
		t.Fatalf("tools/list: %v", r.Result)
	}
	save := list[0].(map[string]any)
	in := save["inputSchema"].(map[string]any)
	props := in["properties"].(map[string]any)
	urlProp := props["url"].(map[string]any)
	tags := props["tags"].(map[string]any)
	if save["title"] != "Save a link" || in["type"] != "object" || in["additionalProperties"] != false ||
		urlProp["format"] != "uri" || urlProp["maxLength"] != float64(2000) || urlProp["description"] != "The page's address" ||
		tags["maxItems"] != float64(5) || tags["items"].(map[string]any)["enum"] == nil ||
		len(in["required"].([]any)) != 1 || save["annotations"].(map[string]any)["destructiveHint"] != false {
		t.Errorf("save_link: %v", save)
	}
	if out := save["outputSchema"].(map[string]any); len(out["required"].([]any)) != 3 {
		t.Errorf("output schema %v", out)
	}

	r = modern(t, u, "tools/call", map[string]any{"name": "save_link", "arguments": map[string]any{"url": "https://go.dev", "title": "Go"}}, nil)
	if r.Status != 200 || r.Result["isError"] != nil || r.Result["structuredContent"].(map[string]any)["title"] != "Go" || !strings.Contains(text(r), `"url":"https://go.dev"`) {
		t.Fatalf("tools/call: %d %v", r.Status, r.Result)
	}

	// Notifications are accepted without a reply.
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": mcp.Modern}}})
	req, _ := http.NewRequest(http.MethodPost, u, bytes.NewReader(b))
	req.Header.Set("MCP-Protocol-Version", mcp.Modern)
	req.Header.Set("Mcp-Method", "notifications/initialized")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Errorf("notification: %d", res.StatusCode)
	}
}

func TestHeadersAndVersions(t *testing.T) {
	u := newApp(t, mcp.Options{}).URL + "/mcp"
	call := map[string]any{"name": "whoami", "arguments": map[string]any{}}

	if r := modern(t, u, "tools/call", call, map[string]string{"Mcp-Name": "save_link"}); r.Status != 400 || r.Error.Code != -32020 {
		t.Errorf("Mcp-Name mismatch: %d %v", r.Status, r.Error)
	}
	if r := modern(t, u, "tools/list", nil, map[string]string{"Mcp-Method": "tools/call"}); r.Status != 400 || r.Error.Code != -32020 {
		t.Errorf("Mcp-Method mismatch: %d %v", r.Status, r.Error)
	}
	if r := modern(t, u, "tools/list", nil, map[string]string{"MCP-Protocol-Version": "2025-06-18"}); r.Status != 400 || r.Error.Code != -32020 {
		t.Errorf("version header mismatch: %d %v", r.Status, r.Error)
	}
	r := post(t, u, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": "1900-01-01"}}},
		map[string]string{"MCP-Protocol-Version": "1900-01-01", "Mcp-Method": "tools/list"})
	if r.Status != 400 || r.Error.Code != -32022 || r.Error.Data["requested"] != "1900-01-01" {
		t.Errorf("unsupported version: %d %v", r.Status, r.Error)
	}
	if r := modern(t, u, "resources/subscribe", nil, nil); r.Status != 404 || r.Error.Code != -32601 {
		t.Errorf("unknown method: %d %v", r.Status, r.Error)
	}
	if r := post(t, u, []any{map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}}, nil); r.Status != 400 {
		t.Errorf("batch: %d", r.Status)
	}
	if r := post(t, u, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}, map[string]string{"Origin": "https://evil.example"}); r.Status != 403 {
		t.Errorf("another origin: %d", r.Status)
	}
	res, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", res.StatusCode)
	}
}

func TestLegacyProtocol(t *testing.T) {
	u := newApp(t, mcp.Options{}).URL + "/mcp"
	r := post(t, u, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "c", "version": "1"}}}, nil)
	if r.Result["protocolVersion"] != "2025-06-18" || r.Result["serverInfo"].(map[string]any)["name"] != "shelf" {
		t.Fatalf("initialize: %v", r.Result)
	}
	// A client asking for a version it can't have gets the newest legacy one.
	if r := post(t, u, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2099-01-01"}}, nil); r.Result["protocolVersion"] != "2025-11-25" {
		t.Errorf("initialize with a future version: %v", r.Result)
	}
	h := map[string]string{"MCP-Protocol-Version": "2025-06-18"}
	if r := post(t, u, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}, h); r.Status != 202 {
		t.Errorf("initialized: %d", r.Status)
	}
	r = post(t, u, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "save_link", "arguments": map[string]any{"url": "https://go.dev"}}}, h)
	if r.Result["resultType"] != nil || r.Result["structuredContent"] == nil {
		t.Errorf("legacy tools/call: %v", r.Result)
	}
	// Before 2025-06-18 there was no header.
	if r := post(t, u, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "ping"}, nil); r.Status != 200 || r.Result == nil {
		t.Errorf("ping without a header: %d %v", r.Status, r.Result)
	}
	if r := post(t, u, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "ping"}, map[string]string{"MCP-Protocol-Version": "2020-01-01"}); r.Status != 400 || r.Error.Code != -32022 {
		t.Errorf("unknown header version: %d %v", r.Status, r.Error)
	}
}

func TestToolErrors(t *testing.T) {
	u := newApp(t, mcp.Options{}).URL + "/mcp"
	call := func(args map[string]any) reply {
		return modern(t, u, "tools/call", map[string]any{"name": "save_link", "arguments": args}, nil)
	}
	for _, c := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"url": "not a url"}, "must be a web address"},
		{map[string]any{"title": "no url"}, "is required"},
		{map[string]any{"url": "https://go.dev", "tags": []string{"cooking"}}, "must be"},
		{map[string]any{"url": "https://go.dev", "extra": "red"}, `unknown field "extra"`},
		{map[string]any{"url": 42}, "must be string"},
		{map[string]any{"url": "https://taken.example"}, "That link is already saved."},
		{map[string]any{"url": "https://boom.example"}, "Internal Error"},
		{map[string]any{"url": "https://panic.example"}, "Internal Error"},
	} {
		r := call(c.args)
		if r.Status != 200 || r.Result["isError"] != true || !strings.Contains(text(r), c.want) {
			t.Errorf("%v: %d %v", c.args, r.Status, r.Result)
		}
		if strings.Contains(text(r), "hunter2") {
			t.Error("an internal error's message reached the client")
		}
	}
	if r := modern(t, u, "tools/call", map[string]any{"name": "whoami"}, nil); text(r) != "This needs a signed-in user." {
		t.Errorf("a redirect: %v", r.Result)
	}
	if r := modern(t, u, "tools/call", map[string]any{"name": "nope"}, nil); r.Error.Code != -32602 {
		t.Errorf("unknown tool: %v", r.Error)
	}
}

func TestAuthorizeOption(t *testing.T) {
	u := newApp(t, mcp.Options{Authorize: func(c *kit.Ctx) error {
		if c.Request.Header.Get("X-API-Key") != "k" {
			return kit.Error(401, "an API key is required")
		}
		c.Locals["user"] = "the key's owner"
		return nil
	}}).URL + "/mcp"
	if r := modern(t, u, "tools/list", nil, nil); r.Status != 401 || r.Error.Message != "an API key is required" {
		t.Errorf("without a key: %d %v", r.Status, r.Error)
	}
	if r := modern(t, u, "tools/call", map[string]any{"name": "whoami"}, map[string]string{"X-API-Key": "k"}); text(r) != "you are the key's owner" {
		t.Errorf("with a key: %v", r.Result)
	}
}

// oauthApp is an app with sessions, a provider and the MCP server, and a
// /login that signs in as alice.
func oauthApp(t *testing.T) (issuer string, provider *oidc.Plugin) {
	t.Helper()
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	issuer = "http://" + l.Addr().String()
	provider = oidc.New(oidc.Options{
		Issuer:   issuer,
		Clients:  []oidc.Client{{ID: "assistant", Public: true, Trusted: true, RedirectURIs: []string{"http://localhost/cb"}}},
		Scopes:   []string{"links:delete"},
		LoginURL: "/login",
		Authenticate: func(ctx *kit.Ctx) (string, error) {
			return sessions.Get(ctx).GetString("user"), nil
		},
	})
	server := mcp.New(mcp.Options{
		Tools: tools(), Name: "shelf",
		OAuth: provider,
		SignIn: func(c *kit.Ctx, tok *oidc.AccessToken) error {
			if tok.Subject == "mallory" {
				return errors.New("suspended")
			}
			c.Locals["user"] = tok.Subject
			return nil
		},
	})
	app, err := kit.New(&kit.Manifest{AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`, Routes: []kit.RouteDef{
		{ID: "/login", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
			if err := sessions.Get(c).Set("user", "alice"); err != nil {
				return err
			}
			return kit.Redirect(http.StatusSeeOther, c.Request.URL.Query().Get("next"))
		}}},
	}}, kit.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		kit.WithPlugins(sessions.New("test-secret-test-secret-test-secret!!"), provider, server))
	if err != nil {
		t.Fatal(err)
	}
	srv := &httptest.Server{Listener: l, Config: &http.Server{Handler: app}}
	srv.Start()
	t.Cleanup(srv.Close)
	return issuer, provider
}

// token runs the authorization code flow with PKCE for resource, as an
// MCP client does, and returns the access token.
func token(t *testing.T, issuer, resource, scope string) string {
	t.Helper()
	conf := &oauth2.Config{ClientID: "assistant", RedirectURL: "http://localhost/cb", Scopes: strings.Fields(scope),
		Endpoint: oauth2.Endpoint{AuthURL: issuer + "/oauth/authorize", TokenURL: issuer + "/oauth/token", AuthStyle: oauth2.AuthStyleInParams}}
	verifier := oauth2.GenerateVerifier()
	jar, _ := cookiejar.New(nil)
	var code string
	b := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		if req.URL.Host == "localhost" {
			code = req.URL.Query().Get("code")
			return http.ErrUseLastResponse
		}
		return nil
	}}
	res, err := b.Get(conf.AuthCodeURL("st", oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("resource", resource)))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if code == "" {
		t.Fatalf("no code: %d", res.StatusCode)
	}
	tok, err := conf.Exchange(context.Background(), code, oauth2.VerifierOption(verifier), oauth2.SetAuthURLParam("resource", resource))
	if err != nil {
		t.Fatal(err)
	}
	return tok.AccessToken
}

func TestOAuth(t *testing.T) {
	issuer, _ := oauthApp(t)
	u := issuer + "/mcp"

	// Without a token: 401, with where to find out how to get one.
	r := modern(t, u, "tools/list", nil, nil)
	challenge := r.Header.Get("WWW-Authenticate")
	if r.Status != 401 || !strings.Contains(challenge, `resource_metadata="`+issuer+`/.well-known/oauth-protected-resource/mcp"`) {
		t.Fatalf("no token: %d %q", r.Status, challenge)
	}
	res, err := http.Get(issuer + "/.well-known/oauth-protected-resource/mcp")
	if err != nil {
		t.Fatal(err)
	}
	var prm map[string]any
	_ = json.NewDecoder(res.Body).Decode(&prm)
	_ = res.Body.Close()
	if prm["resource"] != u || prm["authorization_servers"].([]any)[0] != issuer {
		t.Fatalf("protected resource metadata: %v", prm)
	}

	tok := token(t, issuer, u, "")
	auth := map[string]string{"Authorization": "Bearer " + tok}
	if r := modern(t, u, "tools/call", map[string]any{"name": "whoami"}, auth); text(r) != "you are alice" {
		t.Fatalf("whoami: %d %v", r.Status, r.Result)
	}

	// A token for another resource of the same app is refused.
	other := token(t, issuer, issuer+"/api", "")
	if r := modern(t, u, "tools/list", nil, map[string]string{"Authorization": "Bearer " + other}); r.Status != 401 || !strings.Contains(r.Header.Get("WWW-Authenticate"), `error="invalid_token"`) {
		t.Errorf("a token for another resource: %d", r.Status)
	}
	if r := modern(t, u, "tools/list", nil, map[string]string{"Authorization": "Bearer nonsense"}); r.Status != 401 {
		t.Errorf("a bad token: %d", r.Status)
	}

	// A tool that needs a scope the token lacks: step up.
	r = modern(t, u, "tools/call", map[string]any{"name": "delete_all"}, auth)
	if r.Status != 403 || !strings.Contains(r.Header.Get("WWW-Authenticate"), `error="insufficient_scope", scope="links:delete"`) {
		t.Errorf("missing scope: %d %q", r.Status, r.Header.Get("WWW-Authenticate"))
	}
	scoped := token(t, issuer, u, "links:delete")
	if r := modern(t, u, "tools/call", map[string]any{"name": "delete_all"}, map[string]string{"Authorization": "Bearer " + scoped}); text(r) != "deleted" {
		t.Errorf("with the scope: %d %v", r.Status, r.Result)
	}
}

func TestSetupErrors(t *testing.T) {
	for name, opts := range map[string]mcp.Options{
		"bad name":         {Tools: []mcp.Tool{mcp.NewTool("save link", "", func(*kit.Ctx, Who) (string, error) { return "", nil })}},
		"not a struct":     {Tools: []mcp.Tool{mcp.NewTool("n", "", func(*kit.Ctx, string) (string, error) { return "", nil })}},
		"two of a name":    {Tools: []mcp.Tool{mcp.NewTool("n", "", func(*kit.Ctx, Who) (string, error) { return "", nil }), mcp.NewTool("n", "", func(*kit.Ctx, Who) (string, error) { return "", nil })}},
		"OAuth, no SignIn": {OAuth: oidc.New(oidc.Options{Issuer: "http://localhost"})},
	} {
		_, err := kit.New(&kit.Manifest{AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`}, kit.WithPlugins(mcp.New(opts)))
		if err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	_ = url.Values{}
}
