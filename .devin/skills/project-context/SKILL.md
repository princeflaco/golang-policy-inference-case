---
name: project-context
description: Domain and architecture context for the policy inference service. Use when working on this repo — before changing behavior, naming things, or reasoning about the API contract, the DOT subset, or package boundaries.
---

Service: `POST /infer` executes one deterministic path through a DOT policy
graph and returns input variables enriched with inferred values. Deploys as
an AWS Lambda container image (SAM for local emulation) or a plain HTTP
server when `HTTP_ADDR`/`-http-addr` is set.

## Read first

- Contract (single source of truth for behavior): `docs/specs/policy-inference.md`
- Domain vocabulary: `GLOSSARY.md`
- How it was built: `docs/implementation.md`

Do not infer behavior from the enunciado in `README.md` — the spec supersedes
it. The spec's error-code table and language rules are the contract; changes
to them are contract changes.

## Package map

| Package | Role |
| --- | --- |
| `cmd/infer` | `setup` parses flags, loads config once, builds `httpapi.Service`; `run` starts Lambda (default) or HTTP (`HTTP_ADDR`) |
| `internal/policy` | The engine. `Parse(dot, Limits)` validates the whole policy and compiles each `cond` into typed nodes; `Infer` owns the input contract and runs one path. Reads no env, knows no HTTP. `quoting.go` owns every quote/escape-aware read |
| `internal/httpapi` | One pipeline (`Service`, built by `New(Settings)`) over a transport-neutral `Request`/`Response`; two thin adapters: `Service.Lambda` (API Gateway v2) and `Service.ServeHTTP` (net/http). Body limit, error contract, correlation id and logging live in the pipeline, never in an adapter |
| `internal/apperr` | Shared error contract: `Code` constants (the 8 wire codes) + `Error{Code, Message}` which IS the JSON error body |
| `internal/config` | The only module that reads the environment: `Load(overrides)` with flags > env > `.env` > defaults, validated at load |
| `internal/observability` | correlation id, spans-as-log-events, context logger (`WithLogger` for tests, `slog.Default` fallback) |
| `docs/swagger` | Generated OpenAPI (swag annotations on `httpapi.core`) |

## Conventions

- **Errors**: create with `apperr.New(code, format, ...)`, extract with
  `apperr.From(err)`. Uncoded errors are collapsed to `internal_error` at the
  boundary and logged — never invent ad-hoc error types. `internal/policy`
  must stay free of HTTP concepts (no status codes).
- **Rules**: never log the policy DOT or input values; log codes, counts,
  durations. Errors are logged OR returned, never both.
- **Input contract**: `policy.Infer` alone validates input keys (identifier
  grammar) and value types, returning `invalid_input`. The HTTP decode owns
  only JSON shape — don't re-add input checks there.
- **Configuration**: only `internal/config` reads env/.env; everything else
  receives values by injection (`policy.Limits`, `httpapi.Settings`). Never
  call `config.Load` per request. `policy.DefaultLimits()` is the source of
  default numbers. Keep `template.yaml`, `.env.example` and the README table
  in sync when adding a variable.
- **Transports**: new transport quirks go into an adapter (`lambda.go`,
  `server.go`) that produces a canonical `Request` (canonical path,
  lower-case headers, unread body); new request rules go into the pipeline.
- **Conditions**: grammar changes go in `compileCondition`/`compileOperand`
  only; evaluation nodes must stay unable to meet unsupported syntax.
- **Logging level**: `LOG_LEVEL` (env/.env, default `info`) or `-log-level`.

## Language subset (spec §5)

One directed `digraph`/`strict digraph`; node `result` = literal assignments
(`k=v` comma-separated, quote-aware); edge `cond` = boolean expression with
`==` `!=` `>` `<` `>=` `<=` `&&` `||` parens, same-type comparisons only,
ordering numbers-only, left-to-right short-circuit. No arithmetic, calls,
field access, or collections. Whole policy validated before execution;
failure returns no partial output.
