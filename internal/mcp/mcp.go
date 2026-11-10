// Package mcp is a small Model Context Protocol server over stdio: tools and
// resources, as JSON-RPC 2.0 messages, one per line. It implements what tern
// mcp needs, without a dependency for users' modules.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
)

// Versions are the protocol versions the server speaks, newest first.
var Versions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// Tool is a function a client can call.
type Tool struct {
	Name        string
	Title       string
	Description string
	// Schema is the JSON schema of the arguments, an object.
	Schema map[string]any
	// ReadOnly marks tools that change nothing, for clients that ask
	// before running others.
	ReadOnly bool
	Handler  func(ctx context.Context, args json.RawMessage) (string, error)
}

// Resource is a document a client can read.
type Resource struct {
	URI         string
	Name        string
	Description string
	MimeType    string
	Read        func() (string, error)
}

// Server serves tools and resources.
type Server struct {
	Name, Version string
	// Instructions tell the client's model how to use the server.
	Instructions string
	Tools        []Tool
	Resources    []Resource
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads requests from r and writes responses to w until r ends or
// ctx is canceled.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	var mu sync.Mutex
	write := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		_, err = w.Write(append(b, '\n'))
		return err
	}
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := write(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}}); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 { // a notification
			continue
		}
		result, rerr := s.handle(ctx, req)
		res := response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rerr}
		if rerr == nil && result == nil {
			res.Result = struct{}{}
		}
		if err := write(res); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (s *Server) handle(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := Versions[0]
		if slices.Contains(Versions, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}, "resources": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
			"instructions":    s.Instructions,
		}, nil
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		tools := make([]map[string]any, 0, len(s.Tools))
		for _, t := range s.Tools {
			schema := t.Schema
			if schema == nil {
				schema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			tools = append(tools, map[string]any{
				"name": t.Name, "title": t.Title, "description": t.Description, "inputSchema": schema,
				"annotations": map[string]any{"readOnlyHint": t.ReadOnly, "title": t.Title},
			})
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		i := slices.IndexFunc(s.Tools, func(t Tool) bool { return t.Name == p.Name })
		if i < 0 {
			return nil, &rpcError{-32602, fmt.Sprintf("unknown tool %q", p.Name)}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		text, err := s.call(ctx, s.Tools[i], p.Arguments)
		if err != nil {
			// Tool errors are results, so the model can see and fix them.
			return map[string]any{"content": []map[string]any{{"type": "text", "text": err.Error()}}, "isError": true}, nil
		}
		return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}, nil
	case "resources/list":
		list := make([]map[string]any, 0, len(s.Resources))
		for _, r := range s.Resources {
			list = append(list, map[string]any{"uri": r.URI, "name": r.Name, "description": r.Description, "mimeType": r.MimeType})
		}
		return map[string]any{"resources": list}, nil
	case "resources/templates/list":
		return map[string]any{"resourceTemplates": []any{}}, nil
	case "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		_ = json.Unmarshal(req.Params, &p)
		i := slices.IndexFunc(s.Resources, func(r Resource) bool { return r.URI == p.URI })
		if i < 0 {
			return nil, &rpcError{-32002, "resource not found: " + p.URI}
		}
		text, err := s.Resources[i].Read()
		if err != nil {
			return nil, &rpcError{-32603, err.Error()}
		}
		return map[string]any{"contents": []map[string]any{{"uri": p.URI, "mimeType": s.Resources[i].MimeType, "text": text}}}, nil
	case "prompts/list":
		return map[string]any{"prompts": []any{}}, nil
	default:
		return nil, &rpcError{-32601, "method not found: " + req.Method}
	}
}

// call runs a tool, turning a panic into an error.
func (s *Server) call(ctx context.Context, t Tool, args json.RawMessage) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s failed: %v", t.Name, r)
		}
	}()
	text, err = t.Handler(ctx, args)
	if err == nil && text == "" {
		text = "done"
	}
	return text, err
}

// ErrInvalid wraps argument errors.
var ErrInvalid = errors.New("invalid arguments")

// Args decodes a tool's arguments into T.
func Args[T any](raw json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return v, nil
}
