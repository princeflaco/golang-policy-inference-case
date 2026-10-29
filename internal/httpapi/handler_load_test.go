package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

// load gates the ~70-second HTTP measurement behind an explicit opt-in:
// `go test -run TestHTTPLoad -load ./internal/httpapi` (see `make load-test`).
var load = flag.Bool("load", false, "run the HTTP load measurement")

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

func TestHTTPLoadMediumPolicy(t *testing.T) {
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer slog.SetDefault(previousLogger)

	if !*load {
		t.Skip("pass -load to run the 70-second HTTP load measurement")
	}

	payload, err := json.Marshal(map[string]any{
		"policy_dot": loadPolicyDOT,
		"input": map[string]any{
			"age":     20,
			"score":   700,
			"segment": "prime",
			"active":  true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		response, err := Handle(request.Context(), events.APIGatewayV2HTTPRequest{
			RawPath: request.URL.Path,
			Headers: map[string]string{"Content-Type": request.Header.Get("Content-Type")},
			Body:    string(body),
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
					Method:    request.Method,
					Path:      request.URL.Path,
					Protocol:  request.Proto,
					SourceIP:  request.RemoteAddr,
					UserAgent: request.UserAgent(),
				},
			},
		})
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		for key, value := range response.Headers {
			writer.Header().Set(key, value)
		}
		writer.WriteHeader(response.StatusCode)
		_, _ = writer.Write([]byte(response.Body))
	}))
	defer server.Close()

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
