GO        ?= go
GOFLAGS   ?=
BINARY    := build/infer
SAM       ?= sam
COVER_MIN ?= 90
COVER_PKGS := ./cmd/...,./internal/...

.PHONY: all fmt test test-race vet cover build build-lambda bench load-test perf-container \
	run debug swagger install-hooks check docker-build sam-build sam-local clean help

all: fmt vet test build

# List of available targets (portable; no shell tools required).
help:
	@echo fmt vet test test-race check  - formatting, vet, unit tests
	@echo cover                         - total coverage, fails below COVER_MIN=$(COVER_MIN)%
	@echo build build-lambda            - local binary and linux/amd64 Lambda binary
	@echo run debug                     - local HTTP server; debug also sets log level debug
	@echo bench load-test perf-container- performance evidence
	@echo swagger install-hooks         - regenerate docs/swagger and install git hooks
	@echo docker-build sam-build sam-local clean

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

# Statement coverage across cmd/ and internal/ (generated docs/swagger is
# excluded); fails when the total drops below COVER_MIN percent.
cover:
	$(GO) test -coverpkg=$(COVER_PKGS) -coverprofile=coverage.out ./cmd/... ./internal/...
	@$(GO) tool cover -func=coverage.out | awk -v min=$(COVER_MIN) '/^total:/ { sub("%", "", $$3); printf "total coverage: %s%% (minimum %s%%)\n", $$3, min; exit ($$3 + 0 < min) }'

# Full local verification before committing.
check: fmt vet cover

build:
	$(GO) build -o $(BINARY) ./cmd/infer

# Artifact expected by the Lambda container image (linux/amd64, static).
build-lambda:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -o $(BINARY) ./cmd/infer

# CPU/allocation micro-benchmarks for Parse, Infer and Parse+Infer.
bench:
	$(GO) test -run '^$$' -bench . -benchmem ./internal/policy

# HTTP load test: 50 RPS, 10s warmup + 60s measurement via httptest.
# Gated behind the -load test flag; safe (no Docker containers are spawned).
load-test:
	$(GO) test -run TestHTTPLoad -load -v ./internal/httpapi

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

# Plain HTTP server mode. Flags keep the recipe portable: env prefixes like
# `HTTP_ADDR=:8080 cmd` are POSIX-only and fail under Windows shells.
run:
	$(GO) run ./cmd/infer -http-addr=:8080

# Same as run, with debug-level structured logs.
debug:
	$(GO) run ./cmd/infer -http-addr=:8080 -log-level=debug

# Regenerate the OpenAPI document from swag annotations. Run after changing
# annotations or wire types; the pre-commit hook enforces freshness.
swagger:
	$(GO) tool swag init -g cmd/infer/main.go --parseInternal -o docs/swagger

# Point git at the versioned hooks in scripts/ (one-time per clone).
install-hooks:
	git config core.hooksPath scripts
