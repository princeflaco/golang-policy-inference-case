package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/princeflaco/golang-policy-inference-case/internal/httpapi"
)

func main() {
	// HTTP_ADDR switches the binary into a plain HTTP server. The Lambda
	// image leaves it unset; local development and the containerised perf
	// harness set it (e.g. HTTP_ADDR=:8080).
	if addr := os.Getenv("HTTP_ADDR"); addr != "" {
		slog.Info("starting http server", "addr", addr)
		if err := http.ListenAndServe(addr, httpapi.HTTPHandler()); err != nil {
			log.Fatalf("http server: %v", err)
		}
		return
	}
	lambda.Start(httpapi.Handle)
}
