package codegen

import (
	"encoding/json"
	"fmt"
	"go/types"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/Unfield/Tern/internal/routes"
)

// OpenAPIFile is the generated OpenAPI description of the typed endpoints.
const OpenAPIFile = GenDir + "/generated/openapi.json"

// schemaEmitter converts Go types to JSON Schema (OpenAPI 3.1), following
// encoding/json like tsEmitter. Named structs become components.
type schemaEmitter struct {
	names      map[*types.TypeName]string
	taken      map[string]bool
	components map[string]any
	visiting   map[types.Type]bool
}

func newSchemaEmitter() *schemaEmitter {
	return &schemaEmitter{names: map[*types.TypeName]string{}, taken: map[string]bool{"Error": true}, components: map[string]any{}, visiting: map[types.Type]bool{}}
}

func nullable(s map[string]any) map[string]any {
	if t, ok := s["type"].(string); ok {
		out := map[string]any{}
		for k, v := range s {
			out[k] = v
		}
		out["type"] = []string{t, "null"}
		return out
	}
	return map[string]any{"anyOf": []any{s, map[string]any{"type": "null"}}}
}

func (e *schemaEmitter) schema(t types.Type) map[string]any {
	t = types.Unalias(t)
	if n, ok := t.(*types.Named); ok {
		if obj := n.Obj(); obj.Pkg() != nil {
			switch obj.Pkg().Path() + "." + obj.Name() {
			case "time.Time":
				return map[string]any{"type": "string", "format": "date-time"}
			case "encoding/json.RawMessage":
				return map[string]any{}
			}
		}
	}
	if _, isPtr := t.(*types.Pointer); !isPtr {
		if hasMethod(t, "MarshalJSON") {
			return map[string]any{}
		}
		if hasMethod(t, "MarshalText") {
			return map[string]any{"type": "string"}
		}
	}
	switch u := t.(type) {
	case *types.Named:
		if st, ok := u.Underlying().(*types.Struct); ok && u.TypeArgs().Len() == 0 {
			return map[string]any{"$ref": "#/components/schemas/" + e.component(u, st)}
		}
		if e.visiting[u] {
			return map[string]any{}
		}
		e.visiting[u] = true
		defer delete(e.visiting, u)
		return e.schema(u.Underlying())
	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			return map[string]any{"type": "boolean"}
		case u.Info()&types.IsInteger != 0:
			s := map[string]any{"type": "integer"}
			if u.Kind() == types.Int64 || u.Kind() == types.Uint64 || u.Kind() == types.Int || u.Kind() == types.Uint {
				s["format"] = "int64"
			} else {
				s["format"] = "int32"
			}
			if u.Info()&types.IsUnsigned != 0 {
				s["minimum"] = 0
			}
			return s
		case u.Info()&types.IsFloat != 0:
			return map[string]any{"type": "number"}
		case u.Info()&types.IsString != 0:
			return map[string]any{"type": "string"}
		}
		return map[string]any{}
	case *types.Pointer:
		return nullable(e.schema(u.Elem()))
	case *types.Slice:
		if isByte(u.Elem()) {
			return map[string]any{"type": "string", "contentEncoding": "base64"}
		}
		return nullable(map[string]any{"type": "array", "items": e.schema(u.Elem())})
	case *types.Array:
		return map[string]any{"type": "array", "items": e.schema(u.Elem()), "minItems": u.Len(), "maxItems": u.Len()}
	case *types.Map:
		if !isStringKey(u.Key()) {
			return map[string]any{}
		}
		return nullable(map[string]any{"type": "object", "additionalProperties": e.schema(u.Elem())})
	case *types.Struct:
		return e.object(u)
	}
	return map[string]any{}
}

func (e *schemaEmitter) component(n *types.Named, st *types.Struct) string {
	if name, ok := e.names[n.Obj()]; ok {
		return name
	}
	name := n.Obj().Name()
	if e.taken[name] {
		pkg := n.Obj().Pkg().Name()
		name = strings.ToUpper(pkg[:1]) + pkg[1:] + name
		for i := 2; e.taken[name]; i++ {
			name = fmt.Sprintf("%s%d", n.Obj().Name(), i)
		}
	}
	e.taken[name] = true
	e.names[n.Obj()] = name
	e.components[name] = map[string]any{} // placeholder for recursive types
	e.components[name] = e.object(st)
	return name
}

func (e *schemaEmitter) object(st *types.Struct) map[string]any {
	fields := resolveFields(collectFields(st, 0, map[*types.Struct]bool{}))
	props := map[string]any{}
	var required []string
	for _, f := range fields {
		s := e.schema(f.typ)
		if f.asString {
			s = map[string]any{"type": "string"}
		}
		props[f.name] = s
		if !f.optional {
			required = append(required, f.name)
		}
	}
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		sort.Strings(required)
		out["required"] = required
	}
	return out
}

// inputFields lists an input struct's fields, splitting path parameters
// (path:"name") from the query or body.
func inputFields(in types.Type) (path map[string]string, rest []*types.Var) {
	if in == nil {
		return nil, nil
	}
	st, ok := types.Unalias(in).Underlying().(*types.Struct)
	if !ok {
		return nil, nil
	}
	path = map[string]string{}
	for i := range st.NumFields() {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		if name := reflect.StructTag(st.Tag(i)).Get("path"); name != "" {
			path[name] = jsonName(f, st.Tag(i))
			continue
		}
		rest = append(rest, f)
	}
	return path, rest
}

func jsonName(f *types.Var, tag string) string {
	if n, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ","); n != "" && n != "-" {
		return n
	}
	return f.Name()
}

func queryName(f *types.Var, tag string) string {
	st := reflect.StructTag(tag)
	if n := st.Get("query"); n != "" {
		return n
	}
	return jsonName(f, tag)
}

func bodyMethod(m string) bool { return m != "GET" && m != "HEAD" && m != "DELETE" }

// openAPIPaths returns the OpenAPI paths of a route: optional parameters
// are listed with and without the segment.
func openAPIPaths(r *routes.Route) []struct {
	path   string
	params []routes.Segment
} {
	type variant = struct {
		path   string
		params []routes.Segment
	}
	out := []variant{{}}
	for _, s := range r.Segments {
		var next []variant
		for _, v := range out {
			switch s.Kind {
			case routes.Static:
				next = append(next, variant{v.path + "/" + s.Value, v.params})
			case routes.OptionalParam:
				next = append(next, v, variant{v.path + "/{" + s.Value + "}", append(append([]routes.Segment{}, v.params...), s)})
			default:
				next = append(next, variant{v.path + "/{" + s.Value + "}", append(append([]routes.Segment{}, v.params...), s)})
			}
		}
		out = next
	}
	for i := range out {
		if out[i].path == "" {
			out[i].path = "/"
		}
	}
	return out
}

func operationID(method, path string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	up := true
	for _, r := range path {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if up {
				b.WriteRune(unicode.ToUpper(r))
			} else {
				b.WriteRune(r)
			}
			up = false
		} else {
			up = true
		}
	}
	return b.String()
}

// openAPI writes the OpenAPI description of the typed endpoints.
func (g *generator) openAPI() {
	e := newSchemaEmitter()
	paths := map[string]map[string]any{}
	for _, r := range g.m.Routes {
		rp := g.pkgs[r.Dir]
		if rp == nil || len(rp.endpoints) == 0 {
			continue
		}
		methods := make([]string, 0, len(rp.endpoints))
		for m := range rp.endpoints {
			methods = append(methods, m)
		}
		sort.Strings(methods)
		for _, variant := range openAPIPaths(r) {
			item := paths[variant.path]
			if item == nil {
				item = map[string]any{}
				paths[variant.path] = item
			}
			for _, m := range methods {
				sig := rp.endpoints[m]
				op := map[string]any{"operationId": operationID(m, variant.path)}
				var params []any
				for _, s := range variant.params {
					schema := map[string]any{"type": "string"}
					if s.Matcher == "integer" {
						schema = map[string]any{"type": "integer"}
					}
					p := map[string]any{"name": s.Value, "in": "path", "required": true, "schema": schema}
					if s.Kind == routes.Rest {
						p["description"] = "The rest of the path, which may contain slashes."
					}
					params = append(params, p)
				}
				if sig.in != nil {
					_, fields := inputFields(sig.in)
					st := types.Unalias(sig.in).Underlying().(*types.Struct)
					tags := map[*types.Var]string{}
					for i := range st.NumFields() {
						tags[st.Field(i)] = st.Tag(i)
					}
					if bodyMethod(m) {
						op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": e.schema(sig.in)}}}
					} else {
						for _, f := range fields {
							name := queryName(f, tags[f])
							if name == "-" {
								continue
							}
							s := e.schema(f.Type())
							if _, isSlice := types.Unalias(f.Type()).Underlying().(*types.Slice); isSlice {
								s = map[string]any{"type": "array", "items": e.schema(types.Unalias(f.Type()).Underlying().(*types.Slice).Elem())}
							}
							params = append(params, map[string]any{"name": name, "in": "query", "schema": s})
						}
					}
				}
				if len(params) > 0 {
					op["parameters"] = params
				}
				ok := map[string]any{"description": "OK", "content": map[string]any{"application/json": map[string]any{"schema": e.schema(sig.out)}}}
				if sig.stream {
					// OpenAPI 3.1 can't say what each event is; x-item-schema does.
					ok = map[string]any{"description": "Server-sent events: each event's data is a JSON value of x-item-schema. The stream ends with a done event, or an error event whose data is an Error.", "content": map[string]any{"text/event-stream": map[string]any{
						"schema": map[string]any{"type": "string"}, "x-item-schema": e.schema(sig.out),
					}}}
				}
				op["responses"] = map[string]any{
					"200":     ok,
					"default": map[string]any{"description": "An error", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Error"}}}},
				}
				item[strings.ToLower(m)] = op
			}
		}
	}
	e.components["Error"] = map[string]any{"type": "object", "properties": map[string]any{"message": map[string]any{"type": "string"}}, "required": []string{"message"}}
	doc := map[string]any{
		"openapi":    "3.1.0",
		"info":       map[string]any{"title": "API", "version": "1.0.0"},
		"paths":      paths,
		"components": map[string]any{"schemas": e.components},
	}
	b, _ := json.MarshalIndent(doc, "", "\t")
	g.files[OpenAPIFile] = append(b, '\n')
}
