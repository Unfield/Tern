package telemetry

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// OTLP's JSON shapes for attributes and resources, from
// opentelemetry-proto's common.proto and resource.proto.
type (
	// KeyValue is an attribute.
	KeyValue struct {
		Key   string   `json:"key"`
		Value AnyValue `json:"value"`
	}
	// AnyValue is an attribute's value: one of its fields is set.
	AnyValue struct {
		StringValue *string  `json:"stringValue,omitempty"`
		IntValue    *string  `json:"intValue,omitempty"`
		DoubleValue *float64 `json:"doubleValue,omitempty"`
		BoolValue   *bool    `json:"boolValue,omitempty"`
		KvlistValue *KVList  `json:"kvlistValue,omitempty"`
	}
	// KVList is a group of attributes.
	KVList struct {
		Values []KeyValue `json:"values"`
	}
	// Resource describes what sent the data: service.name and the rest.
	Resource struct {
		Attributes []KeyValue `json:"attributes,omitempty"`
	}
	// Scope names the library that made the data.
	Scope struct {
		Name string `json:"name"`
	}
)

// String is a string value.
func String(s string) AnyValue { return AnyValue{StringValue: &s} }

// Attrs encodes attributes, groups as key-value lists.
func Attrs(attrs []slog.Attr) []KeyValue {
	out := make([]KeyValue, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, KeyValue{Key: a.Key, Value: Value(a.Value)})
	}
	return out
}

// Value encodes a slog value as OTLP's AnyValue.
func Value(v slog.Value) AnyValue {
	switch v.Kind() {
	case slog.KindString:
		return String(v.String())
	case slog.KindInt64:
		s := strconv.FormatInt(v.Int64(), 10)
		return AnyValue{IntValue: &s}
	case slog.KindUint64:
		u := v.Uint64()
		s := strconv.FormatUint(u, 10)
		if u > math.MaxInt64 {
			return AnyValue{StringValue: &s}
		}
		return AnyValue{IntValue: &s}
	case slog.KindFloat64:
		f := v.Float64()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return String(strconv.FormatFloat(f, 'g', -1, 64))
		}
		return AnyValue{DoubleValue: &f}
	case slog.KindBool:
		b := v.Bool()
		return AnyValue{BoolValue: &b}
	case slog.KindDuration:
		return String(v.Duration().String())
	case slog.KindTime:
		return String(v.Time().Format(time.RFC3339Nano))
	case slog.KindGroup:
		return AnyValue{KvlistValue: &KVList{Values: Attrs(v.Group())}}
	case slog.KindLogValuer:
		return Value(v.Resolve())
	}
	switch x := v.Any().(type) {
	case error:
		return String(x.Error())
	case fmt.Stringer:
		return String(x.String())
	}
	b, err := json.Marshal(v.Any())
	if err != nil {
		return String(fmt.Sprint(v.Any()))
	}
	return String(string(b))
}

// Endpoint is where an OTLP signal ("logs", "traces") is posted: explicit,
// or OTEL_EXPORTER_OTLP_<SIGNAL>_ENDPOINT, or OTEL_EXPORTER_OTLP_ENDPOINT
// with /v1/<signal> added, or a collector on localhost.
func Endpoint(explicit, signal string) string {
	if explicit != "" {
		return explicit
	}
	if e := os.Getenv("OTEL_EXPORTER_OTLP_" + strings.ToUpper(signal) + "_ENDPOINT"); e != "" {
		return e
	}
	if base := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); base != "" {
		return strings.TrimRight(base, "/") + "/v1/" + signal
	}
	return "http://localhost:4318/v1/" + signal
}

// Headers are the signal's headers from the environment, then extra.
func Headers(signal string, extra http.Header) http.Header {
	env := os.Getenv("OTEL_EXPORTER_OTLP_" + strings.ToUpper(signal) + "_HEADERS")
	if env == "" {
		env = os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")
	}
	h := ParseHeaders(env)
	for k, vs := range extra {
		h[k] = vs
	}
	return h
}

// NewResource is service.name and the other resource attributes:
// OTEL_RESOURCE_ATTRIBUTES', then extra's, sorted by key. The service is
// named by service, OTEL_SERVICE_NAME, the attributes, or the program.
func NewResource(service string, extra map[string]string) Resource {
	res := map[string]string{}
	for _, p := range ParsePairs(os.Getenv("OTEL_RESOURCE_ATTRIBUTES")) {
		res[p.Key] = p.Value
	}
	for k, v := range extra {
		res[k] = v
	}
	if service == "" {
		service = os.Getenv("OTEL_SERVICE_NAME")
	}
	if service == "" {
		service = res["service.name"]
	}
	if service == "" {
		service = strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	}
	res["service.name"] = service
	keys := make([]string, 0, len(res))
	for k := range res {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var r Resource
	for _, k := range keys {
		r.Attributes = append(r.Attributes, KeyValue{Key: k, Value: String(res[k])})
	}
	return r
}
