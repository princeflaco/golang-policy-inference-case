# Performance report

Date: 2026-10-29 · Branch: `feature/policy-inference`

## Environment

- CPU: AMD Ryzen 5 5500 · go1.26.4 windows/amd64
- All measurements are **local** (engine micro-benchmarks + in-process
  `httptest` HTTP server). They measure engine + transport, not AWS Lambda
  cold start, not API Gateway, and not `sam local` container overhead.

## Workload

Medium policy (see `internal/policy/policy_bench_test.go`): 5 nodes, 6 edges,
mixed conditions (`>=`, `==`, `&&`/`||`), 4 input variables (`age`, `score`,
`segment`, `active`).

## Methodology

```bash
# Micro-benchmarks, 3 repetitions, allocation stats
go test -run '^$' -bench . -benchmem -count=3 ./internal/policy

# HTTP load: 50 RPS offered, 10s warmup + 60s measurement
go test -run TestHTTPLoad -load ./internal/httpapi -v   # or: make load-test
```

## Results — engine benchmarks (median of 3 runs)

| Operation | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `Parse` (medium policy) | ~40 558 | 21 772 | 428 |
| `Infer` | ~1 096 | 368 | 5 |
| `Parse + Infer` | ~38 743 | 22 140 | 433 |

Reading: inference is ~37× cheaper than parsing (1 µs vs 40 µs). A service
that re-parses the same policy per request spends ~97% of its CPU in the
parser — a `policy → *Policy` cache keyed by content hash is the highest-value
optimization if policies repeat across invocations. `Infer` itself allocates
only 5 objects per call (the output map and entries).

## Results — HTTP load test (50 RPS, 60 s)

```json
{
  "offered_rps": 50,
  "achieved_rps": 49.9999,
  "requests": 3000,
  "errors": 0,
  "statuses": {"200": 3000},
  "mean_ms": 0.5034,
  "p50_ms":   0.4679,
  "p90_ms":   0.8744,
  "p99_ms":   1.2483,
  "max_ms":   5.8549
}
```

| Metric | Value | Target | Result |
| --- | ---: | ---: | --- |
| Offered → achieved RPS | 50 → 49.9999 | sustain 50 | pass |
| Errors / non-200 | 0 / 0 | 0 | pass |
| P90 latency | 0.874 ms | < 30 ms | pass (34× headroom) |
| Max latency | 5.85 ms | — | pass |

## Caveats

- **Not Lambda performance.** These numbers exclude container cold start,
  Lambda runtime init, and API Gateway hops. The earlier attempt to measure
  through `sam local start-api` produced invalid data: default mode spawns
  one `rapid-x86_64` container per invocation and saturated the local Docker
  daemon (~1 900 containers, timeouts/502s). Any future SAM-based run must use
  `sam local start-api --warm-containers EAGER` (`make sam-local`).
- Latency is dominated by HTTP round-trip + JSON decode; at 1.1 µs of engine
  work, `Infer` is not the bottleneck — per-request `Parse` is.

## Results — containerised k6 run (neutral environment)

App runs in a distroless container (`tools/perf/Dockerfile`, `HTTP_ADDR=:8080`,
same binary as the Lambda image); the load generator is `grafana/k6` in a
separate container. Reproduce with `make perf-container`.

- 10 s warmup + 60 s measurement at 50 iterations/s (constant-arrival-rate)
- 3 502 requests, 0 failures, 100% checks passed
- http_req_duration: avg 2.15 ms, med 2.04 ms, **p(90) 2.56 ms**, p(95) 2.8 ms, max 15.87 ms
- Thresholds met: `http_req_failed rate==0`, `p(90)<30`

This is the closest neutral measurement available locally: Linux container,
isolated process, external load tool — still excluding Lambda cold start and
API Gateway hops.
