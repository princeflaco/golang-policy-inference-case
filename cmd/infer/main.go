package main

import (
	"log"
	"log/slog"
	"net/http"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/princeflaco/golang-policy-inference-case/internal/config"
	"github.com/princeflaco/golang-policy-inference-case/internal/httpapi"
)

func main() {
	// HTTP_ADDR switches the binary into a plain HTTP server. The Lambda
	// image leaves it unset; local development and the containerised perf
	// harness set it (e.g. HTTP_ADDR=:8080).
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	if addr := cfg.HTTPAddr; addr != "" {
		slog.Info("starting http server", "addr", addr)
		if err := http.ListenAndServe(addr, httpapi.HTTPHandler()); err != nil {
			log.Fatalf("http server: %v", err)
		}
		return
	}
	lambda.Start(httpapi.Handle)
}
