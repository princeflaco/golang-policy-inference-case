// Package config is the only module that reads the process environment.
// Precedence: overrides (command-line flags) > environment variables >
// .env file in the working directory (if present) > defaults. Load is meant
// to run once at startup; the result is injected into the rest of the
// service.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/princeflaco/golang-policy-inference-case/internal/policy"
	"github.com/spf13/viper"
)

// DefaultMaxBodyBytes is the documented request body limit applied when
// MAX_BODY_BYTES is unset.
const DefaultMaxBodyBytes = 1048576

// Setting names, shared by environment variables, .env keys and overrides.
const (
	HTTPAddr           = "HTTP_ADDR"
	LogLevel           = "LOG_LEVEL"
	MaxBodyBytes       = "MAX_BODY_BYTES"
	MaxPolicyNodes     = "MAX_POLICY_NODES"
	MaxPolicyEdges     = "MAX_POLICY_EDGES"
	MaxConditionBytes  = "MAX_CONDITION_BYTES"
	MaxExpressionDepth = "MAX_EXPRESSION_DEPTH"
)

var settings = []string{HTTPAddr, LogLevel, MaxBodyBytes, MaxPolicyNodes, MaxPolicyEdges, MaxConditionBytes, MaxExpressionDepth}

// Config holds every runtime setting of the service.
type Config struct {
	// HTTPAddr switches the binary into a plain HTTP server when set
	// (e.g. ":8080"). Empty means AWS Lambda mode.
	HTTPAddr string `mapstructure:"HTTP_ADDR"`

	// LogLevel selects the slog level: debug, info, warn, or error.
	LogLevel string `mapstructure:"LOG_LEVEL"`

	// MaxBodyBytes bounds the request body size.
	MaxBodyBytes int `mapstructure:"MAX_BODY_BYTES"`

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

// Level returns the parsed log level. Load has already validated it.
func (c *Config) Level() slog.Level {
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.LogLevel))
	return level
}

// Load resolves and validates the effective configuration. overrides maps
// setting names to values with the highest precedence; empty values are
// ignored so unset flags fall through to the environment.
func Load(overrides map[string]string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("env")
	for _, key := range settings {
		if err := v.BindEnv(key); err != nil {
			return nil, err
		}
	}

	defaults := policy.DefaultLimits()
	v.SetDefault(LogLevel, "info")
	v.SetDefault(MaxBodyBytes, DefaultMaxBodyBytes)
	v.SetDefault(MaxPolicyNodes, defaults.MaxPolicyNodes)
	v.SetDefault(MaxPolicyEdges, defaults.MaxPolicyEdges)
	v.SetDefault(MaxConditionBytes, defaults.MaxConditionBytes)
	v.SetDefault(MaxExpressionDepth, defaults.MaxExpressionDepth)

	raw, err := os.ReadFile(".env")
	switch {
	case err == nil:
		if err := v.ReadConfig(bytes.NewReader(raw)); err != nil {
			return nil, fmt.Errorf("read .env: %w", err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("read .env: %w", err)
	}

	for key, value := range overrides {
		if !isSetting(key) {
			return nil, fmt.Errorf("unknown setting %q", key)
		}
		if value != "" {
			v.Set(key, value)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if cfg.MaxBodyBytes <= 0 {
		return nil, fmt.Errorf("%s must be positive", MaxBodyBytes)
	}
	if err := cfg.Limits().Validate(); err != nil {
		return nil, err
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return nil, fmt.Errorf("%s: %w", LogLevel, err)
	}
	return &cfg, nil
}

func isSetting(key string) bool {
	for _, setting := range settings {
		if setting == key {
			return true
		}
	}
	return false
}
