package httpapi

import (
	"context"
	"testing"
)

func TestCorrelationID(t *testing.T) {
	body := payload(t, testPolicy, map[string]any{"age": 20})

	t.Run("echoes incoming id", func(t *testing.T) {
		req := request(body)
		req.Headers["x-correlation-id"] = "corr-123"
		req.RequestID = "gateway-id"
		if got := serve(t, req).Headers["x-correlation-id"]; got != "corr-123" {
			t.Fatalf("x-correlation-id = %q", got)
		}
	})

	t.Run("falls back to the transport request id", func(t *testing.T) {
		req := request(body)
		req.RequestID = "gateway-id"
		if got := serve(t, req).Headers["x-correlation-id"]; got != "gateway-id" {
			t.Fatalf("x-correlation-id = %q", got)
		}
	})

	t.Run("generates id when absent", func(t *testing.T) {
		if got := serve(t, request(body)).Headers["x-correlation-id"]; len(got) != 32 {
			t.Fatalf("x-correlation-id = %q, want a generated 32-hex id", got)
		}
	})

	t.Run("sets headers on responses without any", func(t *testing.T) {
		bare := func(context.Context, Request) Response { return Response{Status: 204} }
		response := Chain(bare, CorrelationID())(context.Background(), Request{Headers: map[string]string{}})
		if response.Headers["x-correlation-id"] == "" {
			t.Fatal("expected x-correlation-id on a header-less response")
		}
	})
}

func TestChainOrdersMiddlewareOutermostFirst(t *testing.T) {
	var order []string
	trace := func(name string) Middleware {
		return func(next Handler) Handler {
			return func(ctx context.Context, req Request) Response {
				order = append(order, name)
				return next(ctx, req)
			}
		}
	}
	handler := Chain(func(context.Context, Request) Response { order = append(order, "handler"); return Response{} }, trace("outer"), trace("inner"))
	handler(context.Background(), Request{})
	if got := len(order); got != 3 || order[0] != "outer" || order[1] != "inner" || order[2] != "handler" {
		t.Fatalf("order = %v", order)
	}
}
