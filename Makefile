GO        ?= go
GOFLAGS   ?=
BINARY    := build/infer
SAM       ?= sam

.PHONY: all fmt test test-race vet build build-lambda bench load-test docker-build sam-build sam-local clean

all: fmt vet test build

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

build:
	$(GO) build -o $(BINARY) ./cmd/infer

# Artifact expected by the Lambda container image (linux/amd64, static).
build-lambda:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -o $(BINARY) ./cmd/infer

# CPU/allocation micro-benchmarks for Parse, Infer and Parse+Infer.
bench:
	$(GO) test -run '^$$' -bench . -benchmem ./internal/policy

# HTTP load test: 50 RPS, 10s warmup + 60s measurement via httptest.
# Requires POLICY_INFER_LOAD_TEST=1; safe (no Docker containers are spawned).
load-test:
	POLICY_INFER_LOAD_TEST=1 $(GO) test -run TestHTTPLoad ./internal/httpapi -v

docker-build:
	docker build -t policy-inference .

sam-build:
	$(SAM) build

# Prefer --warm-containers EAGER under load: default mode spawns one
# container per invocation and can saturate the local Docker daemon.
sam-local:
	$(SAM) local start-api --warm-containers EAGER

clean:
	$(GO) clean
	rm -rf build

# Neutral-environment perf run: app in a distroless container, k6 as the
# load generator. App and generator are separate containers.
perf-container:
	docker build -f tools/perf/Dockerfile -t policy-inference-perf .
	-docker rm -f perf-app
	docker run -d --name perf-app -p 8080:8080 policy-inference-perf
	sleep 2
	docker run --rm -i --add-host=host.docker.internal:host-gateway 		-e BASE_URL=http://host.docker.internal:8080 		grafana/k6 run - < tools/perf/k6.js
	-docker rm -f perf-app

# Plain HTTP server mode (uses .env / env vars via viper).
run:
	HTTP_ADDR=:8080 $(GO) run ./cmd/infer
