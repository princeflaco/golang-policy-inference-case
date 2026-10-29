---
name: dev-workflow
description: How to build, test, run, and verify this repo. Use before running commands, adding config, changing the API contract, or committing — covers the Makefile, swagger regeneration, the pre-commit hook, and what verification is expected.
---

## Commands

`make help` lists targets. The daily ones:

```bash
make check        # fmt + vet + cover — run before finishing any change
make cover        # tests with total coverage of cmd/ + internal/; fails below 90%
make run          # local HTTP server on :8080 (flags, not env — Windows-safe)
make debug        # same, with LOG_LEVEL=debug
make swagger      # regenerate docs/swagger after touching contract or annotations
make bench        # engine micro-benchmarks
make load-test    # 50 RPS / 60s per policy shape via httptest (flag-gated, ~140s for medium + complex)
make sam-local    # SAM emulation (always --warm-containers EAGER)
make cold-start   # cold-start evidence: COLD_START_N invokes, fresh container each (needs sam + docker)
```

`make run`/`debug`/`load-test` use Go flags, not `VAR=x cmd` env prefixes —
env-prefix recipes are POSIX-only and break under Windows shells. Keep new
targets portable the same way.

## After changing the wire contract or swag annotations

Run `make swagger` and **stage `docs/swagger/`**. The versioned pre-commit
hook (`scripts/pre-commit`, enabled with `make install-hooks` /
`git config core.hooksPath scripts`) runs gofmt, vet, `make cover` (tests +
90% coverage floor) and fails if regenerated swagger differs from what's
committed.

The contract docs (`docs/swagger`) are generated artifacts — annotate
handlers, don't hand-edit generated files. There is intentionally no swagger
UI endpoint: the spec rejects any path other than `/infer`.

## Configuration

Precedence: **flags > env > `.env` > defaults**. Runtime vars: `HTTP_ADDR`,
`LOG_LEVEL`, `MAX_BODY_BYTES`, `MAX_POLICY_NODES`, `MAX_POLICY_EDGES`,
`MAX_CONDITION_BYTES`, `MAX_EXPRESSION_DEPTH`. Adding one means touching:
`internal/config/config.go` (setting constant + `settings` list + field),
`.env.example`, `template.yaml`, the README table — and
`internal/policy/limits.go` if it is a policy limit. Config is loaded once
in `cmd/infer/main.go` and injected; tests build `httpapi.Settings` /
`policy.Limits` directly instead of using `t.Setenv`.

## Verification expectations

- `make cover` (≥ 90% total) and `go vet ./...` must pass; `gofmt -l .` must
  be empty. New code ships with tests through the module's interface.
- Error responses must match the spec code table (`internal/apperr`); tests
  in `internal/httpapi` assert `apperr.Code` values through `Service.Serve`
  and both adapters (`adapters_test.go`).
- Capture logs in tests with `observability.WithLogger(ctx, logger)`, not
  `slog.SetDefault`.
- Load test is opt-in: `go test ./internal/httpapi -run TestHTTPLoad -v -args -load`; `-load-shape=medium|complex` picks one payload. Policy shapes come from `internal/policy/policytest` generators — extend them there, not by hand-writing DOT in tests.
- Commits/push/issues only when the user asks (spec §10).
- GitHub Issues for `princeflaco/golang-policy-inference-case` are the
  tracker; see `.devin/docs/agents/issue-tracker.md` before touching them.
