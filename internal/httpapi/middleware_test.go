package httpapi

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCorrelationIDMiddleware(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"policy_dot": testPolicy, "input": map[string]any{"age": 20}})

	t.Run("echoes incoming id", func(t *testing.T) {
		req := request(string(payload))
		req.Headers["x-correlation-id"] = "corr-123"
		resp, err := Handle(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Headers["x-correlation-id"] != "corr-123" {
			t.Fatalf("x-correlation-id = %q", resp.Headers["x-correlation-id"])
		}
	})

	t.Run("generates id when absent", func(t *testing.T) {
		req := request(string(payload))
		resp, err := Handle(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Headers["x-correlation-id"] == "" {
			t.Fatal("expected generated x-correlation-id")
		}
	})
}
