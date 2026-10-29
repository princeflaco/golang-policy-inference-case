package httpapi

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/princeflaco/golang-policy-inference-case/internal/observability"
)

// Handler is the unit middleware composes: a request handler function.
type Handler func(context.Context, events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error)

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
// incoming x-correlation-id, then the API Gateway request id — stores it on
// the context, and echoes it back on the x-correlation-id response header.
func CorrelationID() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
			id := resolveCorrelationID(request)
			ctx = observability.WithCorrelationID(ctx, id)
			response, err := next(ctx, request)
			if response.Headers == nil {
				response.Headers = map[string]string{}
			}
			response.Headers["x-correlation-id"] = id
			return response, err
		}
	}
}

func resolveCorrelationID(request events.APIGatewayV2HTTPRequest) string {
	for key, value := range request.Headers {
		if strings.EqualFold(key, "x-correlation-id") && value != "" {
			return value
		}
	}
	if request.RequestContext.RequestID != "" {
		return request.RequestContext.RequestID
	}
	return observability.NewID()
}

// RequestLogging logs one structured line per request with method, path,
// status, and duration. Business events (parse, inference) are logged inside
// the handler; this middleware only owns the boundary log.
func RequestLogging() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
			started := time.Now()
			response, err := next(ctx, request)
			observability.Logger(ctx).Info("request completed",
				"method", request.RequestContext.HTTP.Method,
				"path", request.RawPath,
				"status", response.StatusCode,
				"duration_ms", float64(time.Since(started).Microseconds())/1000,
			)
			return response, err
		}
	}
}
