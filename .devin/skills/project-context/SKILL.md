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
| `cmd/infer` | Entrypoint: Lambda default, HTTP when `HTTP_ADDR`; flags `-http-addr`, `-log-level` override config |
| `internal/policy` | DOT parsing/validation (Gonum AST), condition language (`go/parser` + restricted evaluator), `result` assignments, inference. Transport-free |
| `internal/httpapi` | API Gateway v2 contract, middleware (correlation id, request logging), `net/http` adapter, `Code → status` mapping |
| `internal/apperr` | Shared error contract: `Code` constants (the 8 wire codes) + `Error{Code, Message}` which IS the JSON error body |
| `internal/config` | viper config: env > `.env` > defaults; `DefaultMaxBodyBytes` |
| `internal/observability` | correlation id, spans-as-log-events, context-aware `slog` logger |
| `docs/swagger` | Generated OpenAPI (swag annotations on `httpapi.core`) |

## Conventions

- **Errors**: create with `apperr.New(code, format, ...)`, extract with
  `apperr.From(err)`. Uncoded errors are collapsed to `internal_error` at the
  boundary and logged — never invent ad-hoc error types. `internal/policy`
  must stay free of HTTP concepts (no status codes).
- **Rules**: never log the policy DOT or input values; log codes, counts,
  durations. Errors are logged OR returned, never both.
- **Identifiers**: nodes, input keys and assignment keys share one grammar —
  `policy.IsIdentifier` (never copy the regex).
- **Limits**: `policy.DefaultLimits()` + `policy.LimitsFromEnv()` are the
  source of numbers; `config` maps them to env vars. Keep `template.yaml`,
  `.env.example` and the README table in sync when adding a variable.
- **Logging level**: `LOG_LEVEL` (env/.env, default `info`) or `-log-level`.

## Language subset (spec §5)

One directed `digraph`/`strict digraph`; node `result` = literal assignments
(`k=v` comma-separated, quote-aware); edge `cond` = boolean expression with
`==` `!=` `>` `<` `>=` `<=` `&&` `||` parens, same-type comparisons only,
ordering numbers-only, left-to-right short-circuit. No arithmetic, calls,
field access, or collections. Whole policy validated before execution;
failure returns no partial output.
