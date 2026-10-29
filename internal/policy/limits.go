package policy

import (
	"os"
	"strconv"
)

const (
	defaultMaxPolicyNodes     = 1000
	defaultMaxPolicyEdges     = 5000
	defaultMaxConditionBytes  = 4096
	defaultMaxExpressionDepth = 64
)

// Limits bounds the resources a single DOT policy may consume at parse time.
// Each field must be positive; see LimitsFromEnv for the resolved defaults.
type Limits struct {
	MaxPolicyNodes     int
	MaxPolicyEdges     int
	MaxConditionBytes  int
	MaxExpressionDepth int
}

// LimitsFromEnv resolves the effective limits from the environment, falling
// back to the documented defaults whenever a variable is unset, malformed,
// or non-positive.
func LimitsFromEnv() Limits {
	return Limits{
		MaxPolicyNodes:     configuredLimit("MAX_POLICY_NODES", defaultMaxPolicyNodes),
		MaxPolicyEdges:     configuredLimit("MAX_POLICY_EDGES", defaultMaxPolicyEdges),
		MaxConditionBytes:  configuredLimit("MAX_CONDITION_BYTES", defaultMaxConditionBytes),
		MaxExpressionDepth: configuredLimit("MAX_EXPRESSION_DEPTH", defaultMaxExpressionDepth),
	}
}

// Option configures Parse. Options are applied in order on top of the
// environment-resolved limits, so later options win.
type Option func(*Limits)

// WithLimits replaces the limits Parse enforces. It exists so callers (tests,
// embedders) can supply explicit bounds instead of relying on the process
// environment.
func WithLimits(l Limits) Option {
	return func(dst *Limits) { *dst = l }
}

func configuredLimit(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
