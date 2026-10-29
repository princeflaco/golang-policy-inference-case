package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/princeflaco/golang-policy-inference-case/internal/observability"
	"github.com/princeflaco/golang-policy-inference-case/internal/policy/policytest"
)

// load gates the ~70-second HTTP measurement behind an explicit opt-in:
// `go test ./internal/httpapi -run TestHTTPLoad -v -args -load` (see `make load-test`).
// -load-shape selects which policy shapes to measure: all, medium, complex.
var (
	load      = flag.Bool("load", false, "run the HTTP load measurement")
	loadShape = flag.String("load-shape", "all", "policy shapes to measure: all, medium, complex")
)

const loadPolicyDOT = `digraph P {
	start [result="tier=base"]
	score [result="score=710"]
	segment [result="segment=\"basic\""]
	high [result="approved=true"]
	low [result="approved=false,reason=\"score\""]

	start -> score [cond="age>=18"]
	start -> segment [cond="age<18"]
	score -> high [cond="score>700 && segment==\"prime\" && active"]
	score -> low [cond="score<=700 || segment==\"basic\""]
	segment -> high [cond="segment==\"prime\" && active"]
	segment -> low [cond="segment==\"basic\""]
}`

type loadResult struct {
	latency time.Duration
	status  int
}

// loadPayloads are the request bodies measured by the load phases. medium is
// the hand-written policy used since the first report; complex is a wide
// layered DAG (37 nodes, 186 edges, ~6 KB body) where Parse does most of the
// per-request work.
var loadPayloads = map[string]map[string]any{
	"medium": {
		"policy_dot": loadPolicyDOT,
		"input": map[string]any{
			"age":     20,
			"score":   700,
			"segment": "prime",
			"active":  true,
		},
	},
	"complex": func() map[string]any {
		shape := policytest.Layered(6, 6)
		return map[string]any{"policy_dot": shape.DOT, "input": shape.Input}
	}(),
}

func TestHTTPLoad(t *testing.T) {
	if !*load {
		t.Skip("pass -load to run the 70-second HTTP load measurement")
	}

	// The measurement drives the production net/http adapter; logs are
	// discarded through the request context instead of global state.
	server := httptest.NewUnstartedServer(newService(t, defaultSettings()))
	server.Config.BaseContext = func(net.Listener) context.Context {
		return observability.WithLogger(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	server.Start()
	defer server.Close()

	for _, name := range []string{"medium", "complex"} {
		if *loadShape != "all" && *loadShape != name {
			continue
		}
		t.Run(name, func(t *testing.T) {
			measureLoad(t, server, loadPayloads[name])
		})
	}
}

func measureLoad(t *testing.T, server *httptest.Server, body map[string]any) {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
		},
	}
	runPhase := func(duration time.Duration) []loadResult {
		requestCount := int(duration.Seconds() * 50)
		results := make(chan loadResult, requestCount)
		next := time.Now()
		for i := 0; i < requestCount; i++ {
			next = next.Add(20 * time.Millisecond)
			if delay := time.Until(next); delay > 0 {
				time.Sleep(delay)
			}
			scheduled := next
			go func() {
				started := time.Now()
				req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/infer", bytes.NewReader(payload))
				if err == nil {
					req.Header.Set("Content-Type", "application/json")
					var response *http.Response
					response, err = client.Do(req)
					if err == nil {
						_, _ = io.Copy(io.Discard, response.Body)
						_ = response.Body.Close()
						results <- loadResult{latency: time.Since(scheduled), status: response.StatusCode}
						return
					}
				}
				results <- loadResult{latency: time.Since(started), status: 0}
			}()
		}
		collected := make([]loadResult, 0, requestCount)
		for i := 0; i < requestCount; i++ {
			collected = append(collected, <-results)
		}
		return collected
	}

	runPhase(10 * time.Second)
	started := time.Now()
	results := runPhase(60 * time.Second)
	elapsed := time.Since(started)

	latencies := make([]float64, 0, len(results))
	statuses := make(map[int]int)
	errors := 0
	for _, result := range results {
		latencies = append(latencies, float64(result.latency)/float64(time.Millisecond))
		statuses[result.status]++
		if result.status < 200 || result.status >= 300 {
			errors++
		}
	}
	sort.Float64s(latencies)
	percentile := func(percent float64) float64 {
		if len(latencies) == 0 {
			return 0
		}
		return latencies[int((float64(len(latencies)-1)*percent)+0.5)]
	}
	report := map[string]any{
		"offered_rps":      50,
		"duration_seconds": elapsed.Seconds(),
		"requests":         len(results),
		"achieved_rps":     float64(len(results)) / elapsed.Seconds(),
		"statuses":         statuses,
		"errors":           errors,
		"p50_ms":           percentile(0.50),
		"p90_ms":           percentile(0.90),
		"p99_ms":           percentile(0.99),
		"mean_ms":          mean(latencies),
		"max_ms":           latencies[len(latencies)-1],
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(encoded))
}

func mean(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}
