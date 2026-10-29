package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/princeflaco/golang-policy-inference-case/internal/config"
	"github.com/princeflaco/golang-policy-inference-case/internal/httpapi"
)

// @title       Policy Inference API
// @version     1.0
// @description Executes one deterministic path through a DOT policy graph and returns the enriched variables.
// @host        localhost:8080
// @BasePath    /
// @schemes     http
func main() {
	// Flags are overrides with the highest precedence. They keep Makefile
	// targets portable: env prefixes like `HTTP_ADDR=:8080 cmd` are
	// POSIX-only.
	httpAddr := flag.String("http-addr", "", "serve plain HTTP on this address instead of the Lambda runtime")
	logLevel := flag.String("log-level", "", "log level: debug, info, warn, error")
	flag.Parse()

	cfg, err := config.Load(map[string]string{config.HTTPAddr: *httpAddr, config.LogLevel: *logLevel})
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.Level()})))

	// HTTP_ADDR switches the binary into a plain HTTP server. The Lambda
	// image leaves it unset; local development and the containerised perf
	// harness set it.
	if cfg.HTTPAddr != "" {
		slog.Info("starting http server", "addr", cfg.HTTPAddr)
		if err := http.ListenAndServe(cfg.HTTPAddr, httpapi.HTTPHandler()); err != nil {
			slog.Error("http server", "error", err)
			os.Exit(1)
		}
		return
	}
	lambda.Start(httpapi.Handle)
}
