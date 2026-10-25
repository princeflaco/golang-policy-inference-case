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

func configuredLimit(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func maxPolicyNodes() int {
	return configuredLimit("MAX_POLICY_NODES", defaultMaxPolicyNodes)
}

func maxPolicyEdges() int {
	return configuredLimit("MAX_POLICY_EDGES", defaultMaxPolicyEdges)
}

func maxConditionBytes() int {
	return configuredLimit("MAX_CONDITION_BYTES", defaultMaxConditionBytes)
}

func maxExpressionDepth() int {
	return configuredLimit("MAX_EXPRESSION_DEPTH", defaultMaxExpressionDepth)
}
