package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/princeflaco/golang-policy-inference-case/internal/apperr"
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

const correlationIDHeader = "x-correlation-id"

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
			response.Headers[correlationIDHeader] = id
			return response, err
		}
	}
}

func resolveCorrelationID(request events.APIGatewayV2HTTPRequest) string {
	for key, value := range request.Headers {
		if strings.EqualFold(key, correlationIDHeader) && value != "" {
			return value
		}
	}
	if request.RequestContext.RequestID != "" {
		return request.RequestContext.RequestID
	}
	return observability.NewID()
}

// RequestLogging logs one structured line per request with method, path,
// status, duration, and — on errors — the wire error code. Business events
// (parse, inference) are logged inside the handler; this middleware only
// owns the boundary log.
func RequestLogging() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
			started := time.Now()
			response, err := next(ctx, request)
			attrs := []any{
				"method", request.RequestContext.HTTP.Method,
				"path", request.RawPath,
				"status", response.StatusCode,
				"duration_ms", float64(time.Since(started).Microseconds()) / 1000,
			}
			if code := responseCode(response); code != "" {
				attrs = append(attrs, "code", code)
			}
			observability.Logger(ctx).Info("request completed", attrs...)
			return response, err
		}
	}
}

// responseCode extracts the wire error code from an error response body;
// successes and bodies without a code yield an empty string.
func responseCode(response events.APIGatewayV2HTTPResponse) string {
	if response.StatusCode < 400 {
		return ""
	}
	var apiErr apperr.Error
	if err := json.Unmarshal([]byte(response.Body), &apiErr); err != nil {
		return ""
	}
	return string(apiErr.Code)
}
