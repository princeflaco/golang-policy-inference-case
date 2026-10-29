// Package observability provides the cross-cutting request context used by
// the service: correlation IDs and lightweight span tracing on top of slog.
//
// The tracer is intentionally minimal: spans are emitted as structured log
// events with span_id/parent_span_id/correlation_id, which keeps the Lambda
// image dependency-free. Swapping StartSpan for an OpenTelemetry tracer is a
// drop-in change confined to this package.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"
)

type contextKey string

const (
	correlationKey contextKey = "correlation-id"
	spanKey        contextKey = "span"
)

// WithCorrelationID stores the request correlation id on the context.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey, id)
}

// CorrelationID returns the correlation id attached to the context, if any.
func CorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(correlationKey).(string); ok {
		return id
	}
	return ""
}

// NewID returns a random 32-hex-character identifier suitable for
// correlation and span ids.
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Logger returns the default logger enriched with the correlation id and the
// current span id, so every downstream log line joins the trace.
func Logger(ctx context.Context) *slog.Logger {
	logger := slog.Default()
	if id := CorrelationID(ctx); id != "" {
		logger = logger.With("correlation_id", id)
	}
	if span := SpanFrom(ctx); span != nil {
		logger = logger.With("span_id", span.id)
	}
	return logger
}

// Span is a timed unit of work emitted as a structured log event.
type Span struct {
	name     string
	id       string
	parentID string
	started  time.Time
}

// StartSpan opens a named span. The returned context carries the span so that
// nested spans become its children and Logger picks up the span id.
func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	span := &Span{name: name, id: NewID(), started: time.Now()}
	if parent := SpanFrom(ctx); parent != nil {
		span.parentID = parent.id
	}
	return context.WithValue(ctx, spanKey, span), span
}

// SpanFrom returns the active span on the context, if any.
func SpanFrom(ctx context.Context) *Span {
	span, _ := ctx.Value(spanKey).(*Span)
	return span
}

// End emits the span completion event with its duration.
func (s *Span) End(ctx context.Context) {
	Logger(ctx).Info("span",
		"span", s.name,
		"span_id", s.id,
		"parent_span_id", s.parentID,
		"duration_ms", float64(time.Since(s.started).Microseconds())/1000,
	)
}
