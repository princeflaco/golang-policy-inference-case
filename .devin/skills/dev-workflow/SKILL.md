---
name: dev-workflow
description: How to build, test, run, and verify this repo. Use before running commands, adding config, changing the API contract, or committing — covers the Makefile, swagger regeneration, the pre-commit hook, and what verification is expected.
---

## Commands

`make help` lists targets. The daily ones:

```bash
make check        # fmt + vet + unit tests — run before finishing any change
make run          # local HTTP server on :8080 (flags, not env — Windows-safe)
make debug        # same, with LOG_LEVEL=debug
make swagger      # regenerate docs/swagger after touching contract or annotations
make bench        # engine micro-benchmarks
make load-test    # 50 RPS / 60s httptest load measurement (flag-gated, ~70s)
make sam-local    # SAM emulation (always --warm-containers EAGER)
```

`make run`/`debug`/`load-test` use Go flags, not `VAR=x cmd` env prefixes —
env-prefix recipes are POSIX-only and break under Windows shells. Keep new
targets portable the same way.

## After changing the wire contract or swag annotations

Run `make swagger` and **stage `docs/swagger/`**. The versioned pre-commit
hook (`scripts/pre-commit`, enabled with `make install-hooks` /
`git config core.hooksPath scripts`) runs gofmt, vet, `go test ./...` and
fails if regenerated swagger differs from what's committed.

The contract docs (`docs/swagger`) are generated artifacts — annotate
handlers, don't hand-edit generated files. There is intentionally no swagger
UI endpoint: the spec rejects any path other than `/infer`.

## Configuration

Precedence: **flags > env > `.env` > defaults**. Runtime vars: `HTTP_ADDR`,
`LOG_LEVEL`, `MAX_BODY_BYTES`, `MAX_POLICY_NODES`, `MAX_POLICY_EDGES`,
`MAX_CONDITION_BYTES`, `MAX_EXPRESSION_DEPTH`. Adding one means touching:
`internal/config/config.go`, `.env.example`, `template.yaml`, the README
table — and `internal/policy/limits.go` if it is a policy limit.

## Verification expectations

- `go test ./...` and `go vet ./...` must pass; `gofmt -l .` must be empty.
- Error responses must match the spec code table (`internal/apperr`); tests
  in `internal/httpapi/handler_test.go` assert `apperr.Code` values.
- Load test is opt-in: `go test -run TestHTTPLoad -load ./internal/httpapi`.
- Commits/push/issues only when the user asks (spec §10).
- GitHub Issues for `princeflaco/golang-policy-inference-case` are the
  tracker; see `.devin/docs/agents/issue-tracker.md` before touching them.
