package policy

import "fmt"

const (
	defaultMaxPolicyNodes     = 1000
	defaultMaxPolicyEdges     = 5000
	defaultMaxConditionBytes  = 4096
	defaultMaxExpressionDepth = 64
)

// Limits bounds the resources a single DOT policy may consume at parse time.
// Every field must be positive. The policy package never reads the process
// environment; callers resolve limits (see internal/config) and pass them in.
type Limits struct {
	MaxPolicyNodes     int
	MaxPolicyEdges     int
	MaxConditionBytes  int
	MaxExpressionDepth int
}

// DefaultLimits returns the documented parse limits.
func DefaultLimits() Limits {
	return Limits{
		MaxPolicyNodes:     defaultMaxPolicyNodes,
		MaxPolicyEdges:     defaultMaxPolicyEdges,
		MaxConditionBytes:  defaultMaxConditionBytes,
		MaxExpressionDepth: defaultMaxExpressionDepth,
	}
}

// Validate reports the first non-positive limit.
func (l Limits) Validate() error {
	for _, field := range []struct {
		name  string
		value int
	}{
		{"MaxPolicyNodes", l.MaxPolicyNodes},
		{"MaxPolicyEdges", l.MaxPolicyEdges},
		{"MaxConditionBytes", l.MaxConditionBytes},
		{"MaxExpressionDepth", l.MaxExpressionDepth},
	} {
		if field.value <= 0 {
			return fmt.Errorf("policy limit %s must be positive, got %d", field.name, field.value)
		}
	}
	return nil
}
