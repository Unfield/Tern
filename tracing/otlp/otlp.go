// Package otlp exports spans to an OpenTelemetry collector, or any service
// that takes OTLP over HTTP (Grafana Tempo, Honeycomb, Jaeger, SigNoz,
// Datadog, New Relic…), as OTLP/HTTP JSON:
//
//	tracing.New(tracing.Options{Exporters: []tracing.Exporter{otlp.New(otlp.Options{})}})
//
// With no options it reads the standard environment variables:
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT,
// OTEL_EXPORTER_OTLP_TRACES_HEADERS or OTEL_EXPORTER_OTLP_HEADERS,
// OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES, as logging/otlp does.
package otlp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Unfield/Tern/internal/telemetry"
	"github.com/Unfield/Tern/tracing"
)

// Options configures the exporter. Unset fields come from the environment.
type Options struct {
	// Endpoint is the URL spans are posted to, ending in /v1/traces.
	Endpoint string
	// Headers are sent with each request, such as an API key.
	Headers http.Header
	// ServiceName is the service.name resource attribute.
	ServiceName string
	// Resource adds resource attributes.
	Resource map[string]string
	// NoGzip sends requests uncompressed.
	NoGzip bool
	// Client sends the requests. The default is http.DefaultClient.
	Client *http.Client
}

// Exporter sends spans over OTLP/HTTP. Make one with New.
type Exporter struct {
	endpoint string
	headers  http.Header
	resource telemetry.Resource
	gzip     bool
	client   *http.Client
}

// New returns an OTLP exporter.
func New(o Options) *Exporter {
	return &Exporter{
		endpoint: telemetry.Endpoint(o.Endpoint, "traces"),
		headers:  telemetry.Headers("traces", o.Headers),
		resource: telemetry.NewResource(o.ServiceName, o.Resource),
		gzip:     !o.NoGzip,
		client:   o.Client,
	}
}

// Name is the exporter's name in error reports.
func (*Exporter) Name() string { return "otlp" }

// Export implements tracing.Exporter.
func (e *Exporter) Export(ctx context.Context, spans []tracing.SpanData) error {
	body, err := json.Marshal(e.request(spans))
	if err != nil {
		return tracing.Permanent(err)
	}
	return telemetry.Do(ctx, telemetry.Request{Client: e.client, URL: e.endpoint, ContentType: "application/json", Header: e.headers, Body: body, Gzip: e.gzip})
}

func nanos(t interface{ UnixNano() int64 }) string { return strconv.FormatInt(t.UnixNano(), 10) }

func (e *Exporter) request(spans []tracing.SpanData) exportRequest {
	out := make([]span, len(spans))
	for i, s := range spans {
		sp := span{
			TraceID:    s.Context.TraceID.String(),
			SpanID:     s.Context.SpanID.String(),
			Name:       s.Name,
			Kind:       int(s.Kind),
			Start:      nanos(s.Start),
			End:        nanos(s.End),
			Attributes: telemetry.Attrs(s.Attrs),
			Status:     status{Code: int(s.Status.Code), Message: s.Status.Message},
		}
		if s.Parent.IsValid() {
			sp.ParentSpanID = s.Parent.String()
		}
		for _, ev := range s.Events {
			sp.Events = append(sp.Events, event{Time: nanos(ev.Time), Name: ev.Name, Attributes: telemetry.Attrs(ev.Attrs)})
		}
		out[i] = sp
	}
	return exportRequest{ResourceSpans: []resourceSpans{{
		Resource:   e.resource,
		ScopeSpans: []scopeSpans{{Scope: telemetry.Scope{Name: "github.com/Unfield/Tern/tracing"}, Spans: out}},
	}}}
}

// The OTLP/HTTP JSON shapes, from opentelemetry-proto's trace.proto.
type (
	exportRequest struct {
		ResourceSpans []resourceSpans `json:"resourceSpans"`
	}
	resourceSpans struct {
		Resource   telemetry.Resource `json:"resource"`
		ScopeSpans []scopeSpans       `json:"scopeSpans"`
	}
	scopeSpans struct {
		Scope telemetry.Scope `json:"scope"`
		Spans []span          `json:"spans"`
	}
	span struct {
		TraceID      string               `json:"traceId"`
		SpanID       string               `json:"spanId"`
		ParentSpanID string               `json:"parentSpanId,omitempty"`
		Name         string               `json:"name"`
		Kind         int                  `json:"kind"`
		Start        string               `json:"startTimeUnixNano"`
		End          string               `json:"endTimeUnixNano"`
		Attributes   []telemetry.KeyValue `json:"attributes,omitempty"`
		Events       []event              `json:"events,omitempty"`
		Status       status               `json:"status"`
	}
	event struct {
		Time       string               `json:"timeUnixNano"`
		Name       string               `json:"name"`
		Attributes []telemetry.KeyValue `json:"attributes,omitempty"`
	}
	status struct {
		Code    int    `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	}
)
