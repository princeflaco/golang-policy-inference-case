package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func capture(t *testing.T) (context.Context, func() []map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	ctx := WithLogger(context.Background(), slog.New(slog.NewJSONHandler(&buf, nil)))
	return ctx, func() []map[string]any {
		var records []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("log line is not JSON: %q", line)
			}
			records = append(records, record)
		}
		return records
	}
}

func TestLoggerUsesContextLoggerAndCorrelationID(t *testing.T) {
	ctx, records := capture(t)
	Logger(WithCorrelationID(ctx, "corr-1")).Info("hello")
	got := records()
	if len(got) != 1 || got[0]["msg"] != "hello" || got[0]["correlation_id"] != "corr-1" {
		t.Fatalf("records = %v", got)
	}
}

func TestNestedSpansLinkParentAndJoinTheTrace(t *testing.T) {
	ctx, records := capture(t)
	ctx = WithCorrelationID(ctx, "corr-2")
	outerCtx, outer := StartSpan(ctx, "outer")
	_, inner := StartSpan(outerCtx, "inner")
	inner.End()
	outer.End()

	got := records()
	if len(got) != 2 {
		t.Fatalf("records = %v", got)
	}
	innerRecord, outerRecord := got[0], got[1]
	if innerRecord["span"] != "inner" || outerRecord["span"] != "outer" {
		t.Fatalf("records = %v", got)
	}
	if innerRecord["parent_span_id"] != outerRecord["span_id"] {
		t.Fatalf("inner parent = %v, outer id = %v", innerRecord["parent_span_id"], outerRecord["span_id"])
	}
	if _, ok := outerRecord["parent_span_id"]; ok {
		t.Fatalf("root span should omit parent_span_id: %v", outerRecord)
	}
	for _, record := range got {
		if record["correlation_id"] != "corr-2" {
			t.Fatalf("span lost correlation id: %v", record)
		}
	}
}

func TestSpanEmitsSpanIDOnce(t *testing.T) {
	var buf bytes.Buffer
	ctx := WithLogger(context.Background(), slog.New(slog.NewTextHandler(&buf, nil)))
	_, span := StartSpan(ctx, "work")
	span.End()
	if n := strings.Count(buf.String(), "span_id="); n != 1 {
		t.Fatalf("span_id appears %d times: %q", n, buf.String())
	}
}
