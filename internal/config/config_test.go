package config

import (
	"log/slog"
	"os"
	"testing"

	"github.com/princeflaco/golang-policy-inference-case/internal/policy"
)

// isolate runs the test in an empty directory (no .env) with every setting
// unset in the environment.
func isolate(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	for _, key := range settings {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func TestLoadDefaults(t *testing.T) {
	isolate(t)
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != "" || cfg.MaxBodyBytes != DefaultMaxBodyBytes || cfg.Level() != slog.LevelInfo {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Limits() != policy.DefaultLimits() {
		t.Fatalf("Limits() = %+v, want defaults", cfg.Limits())
	}
}

func TestLoadPrecedence(t *testing.T) {
	isolate(t)
	if err := os.WriteFile(".env", []byte("MAX_POLICY_NODES=10\nMAX_POLICY_EDGES=20\nLOG_LEVEL=warn\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(MaxPolicyEdges, "30")
	t.Setenv(LogLevel, "error")

	cfg, err := Load(map[string]string{LogLevel: "debug", HTTPAddr: ""})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MaxPolicyNodes != 10 {
		t.Errorf(".env should beat defaults: MaxPolicyNodes = %d", cfg.MaxPolicyNodes)
	}
	if cfg.MaxPolicyEdges != 30 {
		t.Errorf("env should beat .env: MaxPolicyEdges = %d", cfg.MaxPolicyEdges)
	}
	if cfg.Level() != slog.LevelDebug {
		t.Errorf("override should beat env: level = %v", cfg.Level())
	}
	if cfg.HTTPAddr != "" {
		t.Errorf("empty override should fall through: HTTPAddr = %q", cfg.HTTPAddr)
	}
}

func TestLoadRejectsInvalidSettings(t *testing.T) {
	for name, env := range map[string][2]string{
		"zero body":      {MaxBodyBytes, "0"},
		"negative limit": {MaxPolicyNodes, "-1"},
		"not a number":   {MaxPolicyEdges, "many"},
		"bad log level":  {LogLevel, "loud"},
	} {
		t.Run(name, func(t *testing.T) {
			isolate(t)
			t.Setenv(env[0], env[1])
			if _, err := Load(nil); err == nil {
				t.Fatalf("Load() with %s=%s unexpectedly succeeded", env[0], env[1])
			}
		})
	}
}

func TestLoadRejectsUnknownOverride(t *testing.T) {
	isolate(t)
	if _, err := Load(map[string]string{"NOPE": "1"}); err == nil {
		t.Fatal("Load() accepted an unknown override")
	}
}
