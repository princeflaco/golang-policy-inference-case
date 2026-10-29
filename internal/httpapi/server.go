package httpapi

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/princeflaco/golang-policy-inference-case/internal/config"

	"github.com/aws/aws-lambda-go/events"
)

// HTTPHandler adapts the Lambda handler to net/http so the same code can run
// as a plain HTTP server (local development, container perf tests). Lambda
// execution is unaffected.
func HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(maxBody())))
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		headers := make(map[string]string, len(r.Header))
		for key := range r.Header {
			headers[strings.ToLower(key)] = r.Header.Get(key)
		}
		request := events.APIGatewayV2HTTPRequest{
			RawPath: r.URL.Path,
			Body:    string(body),
			Headers: headers,
		}
		request.RequestContext.HTTP.Method = r.Method
		response, err := Handle(context.Background(), request)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for key, value := range response.Headers {
			w.Header().Set(key, value)
		}
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write([]byte(response.Body))
	}
}

func maxBody() int {
	cfg, err := config.Load()
	if err != nil {
		return 1048576
	}
	return cfg.MaxBodyBytes
}
