// Package config externalizes every runtime setting through viper.
// Precedence: environment variables > .env file (if present) > defaults.
// The .env file is read once per process; env vars are read per Load call so
// tests using t.Setenv keep working.
package config

import (
	"bytes"
	"errors"
	"os"
	"sync"

	"github.com/princeflaco/golang-policy-inference-case/internal/policy"
	"github.com/spf13/viper"
)

// DefaultMaxBodyBytes is the documented request body limit applied when
// MAX_BODY_BYTES is unset.
const DefaultMaxBodyBytes = 1048576

// Config holds every runtime setting of the service.
type Config struct {
	// HTTPAddr switches the binary into a plain HTTP server when set
	// (e.g. ":8080"). Empty means AWS Lambda mode.
	HTTPAddr string `mapstructure:"HTTP_ADDR"`

	// MaxBodyBytes bounds the request body size.
	MaxBodyBytes int `mapstructure:"MAX_BODY_BYTES"`

	// LogLevel selects the slog level: debug, info, warn, or error.
	LogLevel string `mapstructure:"LOG_LEVEL"`

	// Policy parse limits; see policy.Limits.
	MaxPolicyNodes     int `mapstructure:"MAX_POLICY_NODES"`
	MaxPolicyEdges     int `mapstructure:"MAX_POLICY_EDGES"`
	MaxConditionBytes  int `mapstructure:"MAX_CONDITION_BYTES"`
	MaxExpressionDepth int `mapstructure:"MAX_EXPRESSION_DEPTH"`
}

// Limits returns the policy parse limits derived from this configuration.
func (c *Config) Limits() policy.Limits {
	return policy.Limits{
		MaxPolicyNodes:     c.MaxPolicyNodes,
		MaxPolicyEdges:     c.MaxPolicyEdges,
		MaxConditionBytes:  c.MaxConditionBytes,
		MaxExpressionDepth: c.MaxExpressionDepth,
	}
}

var (
	dotenvOnce sync.Once
	dotenvRaw  []byte
)

// Load resolves the effective configuration. Missing or non-positive values
// fall back to the documented defaults; .env parse failures are surfaced.
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigType("env")

	for _, key := range []string{
		"HTTP_ADDR", "MAX_BODY_BYTES", "MAX_POLICY_NODES", "MAX_POLICY_EDGES",
		"MAX_CONDITION_BYTES", "MAX_EXPRESSION_DEPTH", "LOG_LEVEL",
	} {
		if err := v.BindEnv(key); err != nil {
			return nil, err
		}
	}
	defaultLimits := policy.DefaultLimits()
	v.SetDefault("MAX_BODY_BYTES", DefaultMaxBodyBytes)
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("MAX_POLICY_NODES", defaultLimits.MaxPolicyNodes)
	v.SetDefault("MAX_POLICY_EDGES", defaultLimits.MaxPolicyEdges)
	v.SetDefault("MAX_CONDITION_BYTES", defaultLimits.MaxConditionBytes)
	v.SetDefault("MAX_EXPRESSION_DEPTH", defaultLimits.MaxExpressionDepth)

	if raw := dotenv(); raw != nil {
		if err := v.ReadConfig(bytes.NewReader(raw)); err != nil {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if cfg.MaxBodyBytes <= 0 {
		return nil, errors.New("MAX_BODY_BYTES must be positive")
	}
	return &cfg, nil
}

// dotenv reads .env once; a missing file is not an error.
func dotenv() []byte {
	dotenvOnce.Do(func() {
		if raw, err := os.ReadFile(".env"); err == nil {
			dotenvRaw = raw
		}
	})
	return dotenvRaw
}
