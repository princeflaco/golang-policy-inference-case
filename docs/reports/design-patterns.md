# Design pattern analysis

Date: 2026-10-29 · Branch: `feature/policy-inference`

This report maps where design patterns fit (and do not fit) in the policy
inference service, and documents the one pattern that was implemented:
**Functional Options** for parse limits.

## Candidate patterns evaluated

| Pattern | Where it could apply | Verdict |
|---|---|---|
| **Functional Options** | `policy.Parse` limits (`internal/policy/limits.go`) | **Implemented.** Before, limits were re-read from env vars inside four helpers on every call, with no way to inject bounds per call. Options make configuration explicit and testable. |
| Strategy | Operator dispatch in `condition.go` (`switch value.Op`) | Rejected. One switch over 6 operators is already simple; a `map[token.Token]func` table would be speculative generality — the spec fixes the operator set. |
| Builder / Template Method | `policy.Parse` pipeline | Rejected. Parse is a short linear pipeline (DOT AST → validate → model). A builder API adds ceremony without a second construction variant. |
| Adapter | `httpapi.Handle` over API Gateway proxy events | Already present by design: `Handle` adapts `events.APIGatewayV2HTTPRequest` to a pure function returning `events.APIGatewayV2HTTPResponse`. Naming it out is enough — no code change needed. |
| Decorator / Middleware | Request logging in `handler.go` | Rejected. A single `slog` call site does not justify a middleware chain in a Lambda entry point. |
| Factory | `Policy` construction | Rejected. `Parse` is the factory; a separate type would duplicate it. |

## Implemented: Functional Options

New API in `internal/policy/limits.go`:

```go
type Limits struct {
    MaxPolicyNodes     int
    MaxPolicyEdges     int
    MaxConditionBytes  int
    MaxExpressionDepth int
}

func LimitsFromEnv() Limits          // env → defaults
type Option func(*Limits)
func WithLimits(l Limits) Option

func Parse(dot string, opts ...Option) (*Policy, error)
```

`Parse` resolves `LimitsFromEnv()` once, applies options in order (later
options win), then enforces the resolved struct. Callers that pass no options
get exactly the previous behavior — `policy.Parse(policyDOT)` in the handler
is unchanged.

Benefits realized:

- **Testability without env mutation**: `Parse(dot, WithLimits(Limits{MaxPolicyNodes: 2, ...}))` exercises limit enforcement directly (`TestParseWithLimitsOption`).
- **Single read of env**: limits were previously looked up up to 6 times per parse; now resolved once.
- **Extensible**: new parser knobs (e.g., max assignment count) become new `Option`s without changing the signature.

Non-positive option values are the caller's responsibility — `WithLimits` is a
power-user hook; `LimitsFromEnv` remains the safe default path.
