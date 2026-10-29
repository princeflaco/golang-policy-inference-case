package httpapi

import (
	"context"
	"time"

	"github.com/princeflaco/golang-policy-inference-case/internal/observability"
)

const correlationIDHeader = "x-correlation-id"

// Handler is the unit middleware composes.
type Handler func(context.Context, Request) Response

// Middleware wraps a Handler with cross-cutting behaviour.
type Middleware func(Handler) Handler

// Chain composes middleware around a handler; the first middleware listed is
// the outermost layer.
func Chain(handler Handler, middlewares ...Middleware) Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// CorrelationID resolves a correlation id for the request — honouring an
// incoming x-correlation-id, then the transport request id — stores it on
// the context, and echoes it back on the x-correlation-id response header.
func CorrelationID() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, request Request) Response {
			id := request.Headers[correlationIDHeader]
			if id == "" {
				id = request.RequestID
			}
			if id == "" {
				id = observability.NewID()
			}
			response := next(observability.WithCorrelationID(ctx, id), request)
			if response.Headers == nil {
				response.Headers = map[string]string{}
			}
			response.Headers[correlationIDHeader] = id
			return response
		}
	}
}

// RequestLogging logs one structured line per request with method, path,
// status, duration, and — on errors — the wire error code. Business events
// (parse, inference) are logged inside the handler; this middleware only
// owns the boundary log.
func RequestLogging() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, request Request) Response {
			started := time.Now()
			response := next(ctx, request)
			attrs := []any{
				"method", request.Method,
				"path", request.Path,
				"status", response.Status,
				"duration_ms", float64(time.Since(started).Microseconds()) / 1000,
			}
			if response.Code != "" {
				attrs = append(attrs, "code", response.Code)
			}
			observability.Logger(ctx).Info("request completed", attrs...)
			return response
		}
	}
}
