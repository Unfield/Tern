package kit

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Typed endpoints are route functions with request and response types,
// which tern gen turns into an OpenAPI description and a typed client:
//
//	type ListQuery struct {
//		Search string `json:"q"`
//		Limit  int    `json:"limit,omitempty"`
//	}
//
//	func GET(ctx *kit.Ctx, in ListQuery) ([]Project, error) { ... }
//
//	type CreateProject struct {
//		Name string `json:"name"`
//	}
//
//	func POST(ctx *kit.Ctx, in CreateProject) (Project, error) { ... }
//
// For GET, HEAD and DELETE the input comes from the query string; for
// other methods it is the JSON body. Fields tagged path:"name" are filled
// from the route's parameters. The input's validate tags are checked, as
// FormAction checks them, and then its Validate() error method if it has
// one; a failure is answered with 400. The output is written as JSON with
// status 200.

// Validator is implemented by inputs that check themselves.
type Validator interface{ Validate() error }

// TypedEndpoint adapts a typed endpoint; generated code calls it.
func TypedEndpoint[In, Out any](method string, fn func(*Ctx, In) (Out, error)) HandlerFunc {
	return func(c *Ctx) error {
		var in In
		if err := decodeInput(c, method, &in); err != nil {
			return err
		}
		if err := checkTyped(c, in); err != nil {
			return err
		}
		if v, ok := any(&in).(Validator); ok {
			if err := v.Validate(); err != nil {
				return asBadRequest(err)
			}
		} else if v, ok := any(in).(Validator); ok {
			if err := v.Validate(); err != nil {
				return asBadRequest(err)
			}
		}
		out, err := fn(c, in)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, out)
	}
}

// OutputEndpoint adapts a typed endpoint without input.
func OutputEndpoint[Out any](fn func(*Ctx) (Out, error)) HandlerFunc {
	return func(c *Ctx) error {
		out, err := fn(c)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, out)
	}
}

func asBadRequest(err error) error {
	var he *HTTPError
	var re *RedirectError
	if errors.As(err, &he) || errors.As(err, &re) {
		return err
	}
	return Error(http.StatusBadRequest, err.Error())
}

func decodeInput(c *Ctx, method string, dst any) error {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodDelete:
		if err := DecodeQuery(c.Request.URL.Query(), dst); err != nil {
			return Error(http.StatusBadRequest, err.Error())
		}
	default:
		ct, _, _ := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
		if ct != "" && ct != "application/json" {
			return Error(http.StatusUnsupportedMediaType, "send the request body as application/json")
		}
		dec := json.NewDecoder(c.Request.Body)
		if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				return Error(http.StatusRequestEntityTooLarge, "Request body too large")
			}
			return Error(http.StatusBadRequest, "invalid JSON body: "+jsonError(err))
		}
	}
	return fillPath(c, dst)
}

func jsonError(err error) string {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return fmt.Sprintf("%s must be %s", te.Field, te.Type)
	}
	return err.Error()
}

// fillPath sets fields tagged path:"name" from route parameters.
func fillPath(c *Ctx, dst any) error {
	v := reflect.ValueOf(dst).Elem()
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	for i := range t.NumField() {
		name := t.Field(i).Tag.Get("path")
		if name == "" || !t.Field(i).IsExported() {
			continue
		}
		if err := setString(v.Field(i), c.Param(name)); err != nil {
			return Error(http.StatusNotFound, "Not Found")
		}
	}
	return nil
}

// DecodeQuery fills the struct dst points to from query parameters, by
// each field's query tag, else its json tag name, else its name. Fields
// may be strings, booleans, numbers, time.Time (RFC 3339), types with
// UnmarshalText, pointers to these (nil when absent), or slices of them
// (from repeated parameters).
func DecodeQuery(q url.Values, dst any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	v = v.Elem()
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() || f.Tag.Get("path") != "" {
			continue
		}
		name := QueryName(f)
		if name == "-" {
			continue
		}
		vals, ok := q[name]
		if !ok || len(vals) == 0 {
			continue
		}
		fv := v.Field(i)
		if fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() != reflect.Uint8 {
			s := reflect.MakeSlice(fv.Type(), len(vals), len(vals))
			for j, raw := range vals {
				if err := setString(s.Index(j), raw); err != nil {
					return fmt.Errorf("query parameter %s: %w", name, err)
				}
			}
			fv.Set(s)
			continue
		}
		if err := setString(fv, vals[0]); err != nil {
			return fmt.Errorf("query parameter %s: %w", name, err)
		}
	}
	return nil
}

// QueryName is the query parameter name of a struct field; see DecodeQuery.
func QueryName(f reflect.StructField) string {
	if n := f.Tag.Get("query"); n != "" {
		return n
	}
	if j, _, _ := strings.Cut(f.Tag.Get("json"), ","); j != "" {
		return j
	}
	return f.Name
}

var textUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()

func setString(v reflect.Value, raw string) error {
	if v.Kind() == reflect.Pointer {
		p := reflect.New(v.Type().Elem())
		if err := setString(p.Elem(), raw); err != nil {
			return err
		}
		v.Set(p)
		return nil
	}
	if v.Type() == reflect.TypeFor[time.Time]() {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return errors.New("must be an RFC 3339 time")
		}
		v.Set(reflect.ValueOf(t))
		return nil
	}
	if v.CanAddr() && v.Addr().Type().Implements(textUnmarshaler) {
		return v.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(raw))
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return errors.New("must be true or false")
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, v.Type().Bits())
		if err != nil {
			return errors.New("must be an integer")
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, v.Type().Bits())
		if err != nil {
			return errors.New("must be a non-negative integer")
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(raw, v.Type().Bits())
		if err != nil {
			return errors.New("must be a number")
		}
		v.SetFloat(n)
	default:
		return fmt.Errorf("unsupported type %s", v.Type())
	}
	return nil
}

// WithOpenAPI serves the OpenAPI description that tern gen writes for the
// app's typed endpoints at path, such as "/openapi.json", with the given
// title and version.
func WithOpenAPI(path, title, version string) Option {
	return func(o *options) { o.openAPI = &openAPIOptions{path: path, title: title, version: version} }
}

type openAPIOptions struct{ path, title, version string }

func (a *App) serveOpenAPI(w http.ResponseWriter) {
	var doc map[string]any
	if err := json.Unmarshal(a.m.OpenAPI, &doc); err != nil {
		http.Error(w, "no OpenAPI description; run tern gen", http.StatusNotFound)
		return
	}
	info, _ := doc["info"].(map[string]any)
	if info == nil {
		info = map[string]any{}
	}
	if a.opts.openAPI.title != "" {
		info["title"] = a.opts.openAPI.title
	}
	if a.opts.openAPI.version != "" {
		info["version"] = a.opts.openAPI.version
	}
	doc["info"] = info
	if a.m.BasePath != "" {
		doc["servers"] = []map[string]string{{"url": a.m.BasePath}}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(doc)
}
