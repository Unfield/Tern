package mcp

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/Unfield/Tern/internal/jsonenc"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/plugins/oidc"
)

const tokenKey = "mcp.token"

// handle runs a method. A nil result and error means the response was
// written already.
func (s *Server) handle(c *kit.Ctx, version string, req *rpcRequest) (map[string]any, *rpcError, int) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := legacy[0]
		if slices.Contains(legacy, p.ProtocolVersion) {
			v = p.ProtocolVersion
		}
		res := map[string]any{
			"protocolVersion": v,
			"capabilities":    s.capabilities(),
			"serverInfo":      s.serverInfo(),
		}
		if s.opts.Instructions != "" {
			res["instructions"] = s.opts.Instructions
		}
		return res, nil, 0
	case "server/discover":
		res := map[string]any{
			"supportedVersions": Versions,
			"capabilities":      s.capabilities(),
			"ttlMs":             3600000,
			"cacheScope":        "public",
		}
		if s.opts.Instructions != "" {
			res["instructions"] = s.opts.Instructions
		}
		return res, nil, 0
	case "ping":
		return map[string]any{}, nil, 0
	case "tools/list":
		tools := make([]map[string]any, len(s.tools))
		for i := range s.tools {
			tools[i] = s.tools[i].definition()
		}
		res := map[string]any{"tools": tools}
		if version == Modern {
			res["ttlMs"] = 300000
			res["cacheScope"] = "public"
		}
		return res, nil, 0
	case "tools/call":
		return s.call(c, version, req)
	default:
		status := http.StatusOK
		if version == Modern {
			status = http.StatusNotFound
		}
		return nil, &rpcError{Code: codeMethodNotFound, Message: "Method not found: " + req.Method}, status
	}
}

func (s *Server) capabilities() map[string]any {
	return map[string]any{"tools": map[string]any{}}
}

func (s *Server) call(c *kit.Ctx, version string, req *rpcRequest) (map[string]any, *rpcError, int) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params"}, http.StatusOK
	}
	t := s.byName[p.Name]
	if t == nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "Unknown tool: " + p.Name}, http.StatusOK
	}
	if len(t.scopes) > 0 {
		tok, _ := c.Locals[tokenKey].(*oidc.AccessToken)
		if tok != nil {
			if missing := missingScopes(tok, t.scopes); len(missing) > 0 {
				s.challenge(c, http.StatusForbidden, "insufficient_scope", append(slices.Clone(s.opts.Scopes), t.scopes...))
				return nil, nil, 0
			}
		}
	}
	args := p.Arguments
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	}
	out, err := s.run(c, t, args)
	if err != nil {
		return map[string]any{"content": []map[string]any{{"type": "text", "text": s.message(c, err)}}, "isError": true}, nil, 0
	}
	b, err := jsonenc.Marshal(out)
	if err != nil {
		return map[string]any{"content": []map[string]any{{"type": "text", "text": s.message(c, err)}}, "isError": true}, nil, 0
	}
	text := string(b)
	if str, ok := out.(string); ok {
		text = str
	}
	res := map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
	// Earlier versions allow only an object here; 2026-07-28 any JSON. A
	// string result is its text.
	if _, isText := out.(string); !isText && (version == Modern || (len(b) > 0 && b[0] == '{')) {
		res["structuredContent"] = json.RawMessage(b)
	}
	return res, nil, 0
}

// run calls the tool, turning a panic into an error.
func (s *Server) run(c *kit.Ctx, t *Tool, args json.RawMessage) (out any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("mcp: tool %s panicked: %v", t.name, r)
		}
	}()
	return t.call(c, args)
}

// message is what the assistant sees of a tool's error.
func (s *Server) message(c *kit.Ctx, err error) string {
	var re *kit.RedirectError
	if errors.As(err, &re) {
		// Most often accounts' Require, for a request without a user.
		return "This needs a signed-in user."
	}
	_, msg := c.UserError(err)
	return msg
}

func missingScopes(t *oidc.AccessToken, want []string) []string {
	var out []string
	for _, sc := range want {
		if !t.HasScope(sc) {
			out = append(out, sc)
		}
	}
	return out
}

// authorize checks the request's credentials. When it refuses, it has
// written the response.
func (s *Server) authorize(c *kit.Ctx, scopes []string) (bool, error) {
	switch {
	case s.opts.OAuth != nil:
		raw, ok := strings.CutPrefix(c.Request.Header.Get("Authorization"), "Bearer ")
		if !ok || strings.TrimSpace(raw) == "" {
			s.challenge(c, http.StatusUnauthorized, "", scopes)
			return false, nil
		}
		tok, err := s.opts.OAuth.Verify(c, strings.TrimSpace(raw))
		if errors.Is(err, oidc.ErrInvalidToken) || (err == nil && !tok.For(s.opts.Resource)) {
			// A token for another resource is refused like a bad one: the
			// server must never accept tokens issued for something else.
			s.challenge(c, http.StatusUnauthorized, "invalid_token", scopes)
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if len(missingScopes(tok, scopes)) > 0 {
			s.challenge(c, http.StatusForbidden, "insufficient_scope", scopes)
			return false, nil
		}
		if err := s.opts.SignIn(c, tok); err != nil {
			c.Logger().Info("mcp: refused a token's user", "subject", tok.Subject, "err", err)
			s.challenge(c, http.StatusUnauthorized, "invalid_token", scopes)
			return false, nil
		}
		c.Locals[tokenKey] = tok
	case s.opts.Authorize != nil:
		if err := s.opts.Authorize(c); err != nil {
			status, msg := c.UserError(err)
			s.write(c.Writer(), status, nil, nil, &rpcError{Code: codeInvalidRequest, Message: msg})
			return false, nil
		}
	}
	return true, nil
}

// challenge answers 401 or 403 with the WWW-Authenticate header MCP
// clients follow to authorize (RFC 6750, RFC 9728).
func (s *Server) challenge(c *kit.Ctx, status int, code string, scopes []string) {
	parts := []string{fmt.Sprintf("resource_metadata=%q", s.metadata)}
	if code != "" {
		parts = append(parts, fmt.Sprintf("error=%q", code))
	}
	if len(scopes) > 0 {
		parts = append(parts, fmt.Sprintf("scope=%q", strings.Join(scopes, " ")))
	}
	c.Writer().Header().Set("WWW-Authenticate", "Bearer "+strings.Join(parts, ", "))
	msg := "Unauthorized"
	if status == http.StatusForbidden {
		msg = "Forbidden: the token lacks a scope"
	}
	s.write(c.Writer(), status, nil, nil, &rpcError{Code: codeInvalidRequest, Message: msg})
}

func decodeBase64(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}
