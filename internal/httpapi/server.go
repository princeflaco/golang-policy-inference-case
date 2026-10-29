package httpapi

import (
	"net/http"
	"strings"
)

// ServeHTTP adapts the service to net/http, so the same pipeline runs as a
// plain HTTP server (local development, container perf runs, load tests).
// The body is passed through unread: the pipeline enforces the size limit
// and answers with the contract's JSON errors on every transport.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	headers := make(map[string]string, len(r.Header))
	for key := range r.Header {
		headers[strings.ToLower(key)] = r.Header.Get(key)
	}
	response := s.Serve(r.Context(), Request{
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: headers,
		Body:    r.Body,
	})
	for key, value := range response.Headers {
		w.Header().Set(key, value)
	}
	w.WriteHeader(response.Status)
	_, _ = w.Write(response.Body)
}
