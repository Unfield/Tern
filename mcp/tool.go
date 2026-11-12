package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"

	"github.com/Unfield/Tern/kit"
)

// Tool is a function an assistant can call. Make one with NewTool.
type Tool struct {
	name, title, description string
	input, output            map[string]any
	annotations              map[string]any
	scopes                   []string
	call                     func(c *kit.Ctx, args json.RawMessage) (any, error)
	err                      error
}

// ToolOption describes a tool further.
type ToolOption func(*Tool)

// Title is the tool's name for people, such as "Save a link".
func Title(title string) ToolOption { return func(t *Tool) { t.title = title } }

// ReadOnly marks a tool that changes nothing, such as a search, so
// assistants may call it without asking the user first.
func ReadOnly() ToolOption {
	return func(t *Tool) { t.annotations["readOnlyHint"] = true }
}

// Destructive marks a tool that deletes or overwrites, so assistants ask
// before calling it. Tools that aren't ReadOnly are assumed destructive
// unless marked Additive.
func Destructive() ToolOption {
	return func(t *Tool) { t.annotations["destructiveHint"] = true }
}

// Additive marks a tool that only adds, such as saving a new link.
func Additive() ToolOption {
	return func(t *Tool) { t.annotations["destructiveHint"] = false }
}

// Idempotent marks a tool that does nothing more when called again with
// the same arguments, such as marking something read.
func Idempotent() ToolOption {
	return func(t *Tool) { t.annotations["idempotentHint"] = true }
}

// Scopes are the OAuth scopes a token needs to call the tool. A token
// without them gets 403 with the scopes to ask the user for.
func Scopes(scopes ...string) ToolOption {
	return func(t *Tool) { t.scopes = append(t.scopes, scopes...) }
}

var toolName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// NewTool makes a tool of fn. In is the arguments, a struct: its json
// names, `description` tags and validate rules become the input schema,
// and the rules are checked before fn runs, as for a typed endpoint. Out
// is the result, sent as JSON. fn gets the request's Ctx, with the user
// the token belongs to signed in, so it can use the app's own checks:
//
//	mcp.NewTool("save_link", "Save a link to read later.",
//		func(ctx *kit.Ctx, in NewLink) (shelf.Link, error) {
//			user, err := shelf.Accounts.Require(ctx)
//			if err != nil {
//				return shelf.Link{}, err
//			}
//			return shelf.Add(ctx, user.ID, in.URL, in.Title)
//		}, mcp.Additive())
//
// Errors from fn are shown to the assistant, so it can correct itself:
// those from kit.Error, kit.NotFound and failed validation with their
// message, others as "Internal Error" (and logged).
func NewTool[In, Out any](name, description string, fn func(*kit.Ctx, In) (Out, error), opts ...ToolOption) Tool {
	t := Tool{name: name, description: description, annotations: map[string]any{"openWorldHint": false}}
	for _, o := range opts {
		o(&t)
	}
	if t.title != "" {
		t.annotations["title"] = t.title
	}
	in, out := reflect.TypeFor[In](), reflect.TypeFor[Out]()
	base := in
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	switch {
	case !toolName.MatchString(name):
		t.err = fmt.Errorf("mcp: tool name %q: use 1 to 128 of letters, digits and the characters '_', '-' and '.'", name)
	case base.Kind() != reflect.Struct:
		t.err = fmt.Errorf("mcp: tool %s: the arguments must be a struct, not %s", name, in)
	}
	t.input = schemaOf(in, true)
	if o := out; o.Kind() == reflect.Struct || (o.Kind() == reflect.Pointer && o.Elem().Kind() == reflect.Struct) {
		t.output = schemaOf(out, false)
	}
	t.call = func(c *kit.Ctx, args json.RawMessage) (any, error) {
		var v In
		dec := json.NewDecoder(bytes.NewReader(args))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&v); err != nil {
			return nil, kit.Error(400, "Invalid arguments: "+argumentError(err))
		}
		if err := kit.ValidateInput(c, &v); err != nil {
			return nil, err
		}
		return fn(c, v)
	}
	return t
}

func argumentError(err error) string {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return fmt.Sprintf("%s must be %s", te.Field, te.Type)
	}
	return err.Error()
}

// definition is the tool as tools/list describes it.
func (t *Tool) definition() map[string]any {
	d := map[string]any{"name": t.name, "description": t.description, "inputSchema": t.input, "annotations": t.annotations}
	if t.title != "" {
		d["title"] = t.title
	}
	if t.output != nil {
		d["outputSchema"] = t.output
	}
	return d
}
