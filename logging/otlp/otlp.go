// Package otlp is a logging sink that sends records to an OpenTelemetry
// collector, or any service that takes OTLP over HTTP (Grafana Cloud,
// Honeycomb, New Relic, SigNoz, Axiom…), as OTLP/HTTP JSON:
//
//	logging.New(logging.Options{Sinks: []logging.Sink{otlp.New(otlp.Options{})}})
//
// With no options it reads the standard environment variables:
// OTEL_EXPORTER_OTLP_LOGS_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT,
// OTEL_EXPORTER_OTLP_LOGS_HEADERS or OTEL_EXPORTER_OTLP_HEADERS,
// OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES. Records logged in a
// request with a trusted traceparent carry its trace and span.
package otlp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Unfield/Tern/internal/telemetry"
	"github.com/Unfield/Tern/logging"
)

// Options configures the sink. Unset fields come from the environment.
type Options struct {
	// Endpoint is the URL logs are posted to, ending in /v1/logs. The
	// default is OTEL_EXPORTER_OTLP_LOGS_ENDPOINT's, or
	// OTEL_EXPORTER_OTLP_ENDPOINT's with /v1/logs added, or
	// http://localhost:4318/v1/logs.
	Endpoint string
	// Headers are sent with each request, such as an API key.
	Headers http.Header
	// ServiceName is the service.name resource attribute. The default is
	// OTEL_SERVICE_NAME's, or the program's name.
	ServiceName string
	// Resource adds resource attributes, after OTEL_RESOURCE_ATTRIBUTES'.
	Resource map[string]string
	// NoGzip sends requests uncompressed.
	NoGzip bool
	// Client sends the requests. The default is http.DefaultClient.
	Client *http.Client
}

// Sink sends records over OTLP/HTTP. Make one with New.
type Sink struct {
	endpoint string
	headers  http.Header
	resource telemetry.Resource
	gzip     bool
	client   *http.Client
}

// New returns an OTLP sink.
func New(o Options) *Sink {
	return &Sink{
		endpoint: telemetry.Endpoint(o.Endpoint, "logs"),
		headers:  telemetry.Headers("logs", o.Headers),
		resource: telemetry.NewResource(o.ServiceName, o.Resource),
		gzip:     !o.NoGzip,
		client:   o.Client,
	}
}

// Name implements the name logging reports errors with.
func (*Sink) Name() string { return "otlp" }

// Export implements logging.Sink.
func (s *Sink) Export(ctx context.Context, entries []logging.Entry) error {
	body, err := json.Marshal(s.request(entries))
	if err != nil {
		return logging.Permanent(err)
	}
	return telemetry.Do(ctx, telemetry.Request{Client: s.client, URL: s.endpoint, ContentType: "application/json", Header: s.headers, Body: body, Gzip: s.gzip})
}

func (s *Sink) request(entries []logging.Entry) exportRequest {
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	recs := make([]logRecord, len(entries))
	for i, e := range entries {
		r := logRecord{
			TimeUnixNano:         strconv.FormatInt(e.Time.UnixNano(), 10),
			ObservedTimeUnixNano: now,
			SeverityNumber:       Severity(e.Level),
			SeverityText:         e.Level.String(),
			Body:                 telemetry.String(e.Message),
		}
		if e.Time.IsZero() {
			r.TimeUnixNano = "0"
		}
		for _, a := range e.Attrs {
			switch a.Key {
			case logging.TraceIDKey:
				if v := a.Value.String(); len(v) == 32 {
					r.TraceID = v
					continue
				}
			case logging.SpanIDKey:
				if v := a.Value.String(); len(v) == 16 {
					r.SpanID = v
					continue
				}
			}
			r.Attributes = append(r.Attributes, telemetry.KeyValue{Key: a.Key, Value: telemetry.Value(a.Value)})
		}
		if src := e.Source; src != nil {
			r.Attributes = append(r.Attributes,
				telemetry.KeyValue{Key: "code.file.path", Value: telemetry.String(src.File)},
				telemetry.KeyValue{Key: "code.line.number", Value: telemetry.Value(slog.IntValue(src.Line))},
				telemetry.KeyValue{Key: "code.function.name", Value: telemetry.String(src.Function)},
			)
		}
		recs[i] = r
	}
	return exportRequest{ResourceLogs: []resourceLogs{{
		Resource:  s.resource,
		ScopeLogs: []scopeLogs{{Scope: telemetry.Scope{Name: "github.com/Unfield/Tern/logging"}, LogRecords: recs}},
	}}}
}

// Severity is OpenTelemetry's severity number for a slog level: DEBUG is
// 5, INFO 9, WARN 13 and ERROR 17, with the levels between in between.
func Severity(l slog.Level) int {
	return min(max(int(l)+9, 1), 24)
}

// The OTLP/HTTP JSON shapes, from opentelemetry-proto's logs.proto.
type (
	exportRequest struct {
		ResourceLogs []resourceLogs `json:"resourceLogs"`
	}
	resourceLogs struct {
		Resource  telemetry.Resource `json:"resource"`
		ScopeLogs []scopeLogs        `json:"scopeLogs"`
	}
	scopeLogs struct {
		Scope      telemetry.Scope `json:"scope"`
		LogRecords []logRecord     `json:"logRecords"`
	}
	logRecord struct {
		TimeUnixNano         string               `json:"timeUnixNano"`
		ObservedTimeUnixNano string               `json:"observedTimeUnixNano"`
		SeverityNumber       int                  `json:"severityNumber"`
		SeverityText         string               `json:"severityText"`
		Body                 telemetry.AnyValue   `json:"body"`
		Attributes           []telemetry.KeyValue `json:"attributes,omitempty"`
		TraceID              string               `json:"traceId,omitempty"`
		SpanID               string               `json:"spanId,omitempty"`
	}
)
