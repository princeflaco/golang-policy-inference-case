package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/princeflaco/golang-policy-inference-case/internal/httpapi"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
}

func TestSetupAppliesFlagsAndBuildsAWorkingService(t *testing.T) {
	isolate(t)
	t.Setenv("HTTP_ADDR", ":1")
	var logs bytes.Buffer
	app, err := setup([]string{"-http-addr=:9090", "-log-level=debug"}, &logs)
	if err != nil {
		t.Fatalf("setup() error = %v", err)
	}
	if app.httpAddr != ":9090" {
		t.Fatalf("httpAddr = %q, flag should override env", app.httpAddr)
	}
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("-log-level=debug did not reach the default logger")
	}
	response := app.service.Serve(context.Background(), httpapi.Request{
		Method:  "POST",
		Path:    "/infer",
		Headers: map[string]string{"content-type": "application/json"},
		Body:    strings.NewReader(`{"policy_dot":"digraph { start [result=\"ok=true\"] }","input":{}}`),
	})
	if response.Status != 200 {
		t.Fatalf("Status = %d, body = %s", response.Status, response.Body)
	}
}

func TestSetupDefaultsToLambdaMode(t *testing.T) {
	isolate(t)
	t.Setenv("HTTP_ADDR", "")
	app, err := setup(nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("setup() error = %v", err)
	}
	if app.httpAddr != "" {
		t.Fatalf("httpAddr = %q, want Lambda mode", app.httpAddr)
	}
}

func TestSetupFailures(t *testing.T) {
	for name, args := range map[string][]string{
		"unknown flag":      {"-nope"},
		"invalid log level": {"-log-level=loud"},
	} {
		t.Run(name, func(t *testing.T) {
			isolate(t)
			if _, err := setup(args, &bytes.Buffer{}); err == nil {
				t.Fatal("setup() unexpectedly succeeded")
			}
		})
	}
}

func TestRunReportsHTTPServerErrors(t *testing.T) {
	isolate(t)
	app, err := setup([]string{"-http-addr=not-an-address"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("setup() error = %v", err)
	}
	if err := app.run(); err == nil {
		t.Fatal("run() with an invalid address unexpectedly succeeded")
	}
}
