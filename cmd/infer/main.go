package main

import (
	"flag"
	"io"
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
	app, err := setup(os.Args[1:], os.Stderr)
	if err == nil {
		err = app.run()
	}
	if err != nil {
		slog.Error("infer", "error", err)
		os.Exit(1)
	}
}

type app struct {
	httpAddr string
	service  *httpapi.Service
}

// run serves until the process ends. HTTP_ADDR switches the binary into a
// plain HTTP server; the Lambda image leaves it unset, while local
// development and the containerised perf harness set it.
func (a *app) run() error {
	if a.httpAddr == "" {
		lambda.Start(a.service.Lambda)
		return nil
	}
	slog.Info("starting http server", "addr", a.httpAddr)
	return http.ListenAndServe(a.httpAddr, a.service)
}

// setup parses flags, loads configuration once, installs the default logger
// writing to logOutput, and builds the service. Flags are overrides with the
// highest precedence; they keep Makefile targets portable, since env
// prefixes like `HTTP_ADDR=:8080 cmd` are POSIX-only.
func setup(args []string, logOutput io.Writer) (*app, error) {
	flags := flag.NewFlagSet("infer", flag.ContinueOnError)
	flags.SetOutput(logOutput)
	httpAddr := flags.String("http-addr", "", "serve plain HTTP on this address instead of the Lambda runtime")
	logLevel := flags.String("log-level", "", "log level: debug, info, warn, error")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	cfg, err := config.Load(map[string]string{config.HTTPAddr: *httpAddr, config.LogLevel: *logLevel})
	if err != nil {
		return nil, err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: cfg.Level()})))

	service, err := httpapi.New(httpapi.Settings{MaxBodyBytes: cfg.MaxBodyBytes, Limits: cfg.Limits()})
	if err != nil {
		return nil, err
	}
	return &app{httpAddr: cfg.HTTPAddr, service: service}, nil
}
