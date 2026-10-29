package httpapi

import (
	"context"
	"errors"
	"io"

	"github.com/princeflaco/golang-policy-inference-case/internal/apperr"
	"github.com/princeflaco/golang-policy-inference-case/internal/policy"
)

// Settings is everything the inference pipeline needs from configuration.
// The caller resolves it once at startup (see internal/config); the pipeline
// never reads the environment.
type Settings struct {
	MaxBodyBytes int
	Limits       policy.Limits
}

// Request is the transport-neutral request every adapter produces.
//
// Invariants adapters must uphold: Path is canonical (no API Gateway stage
// prefix), header names are lower-case, and Body is handed over unread and
// unlimited — the pipeline reads it once and enforces MaxBodyBytes, so every
// transport answers oversized bodies with the same 413 request_too_large.
// A Body read error carrying an apperr code is reported with that code.
type Request struct {
	Method    string
	Path      string
	Headers   map[string]string
	Body      io.Reader
	RequestID string // transport-assigned id; correlation fallback
}

// Response is what the pipeline returns to an adapter. Code is the wire
// error code ("" on success), so middleware never re-parses Body.
type Response struct {
	Status  int
	Headers map[string]string
	Body    []byte
	Code    apperr.Code
}

// Service is the POST /infer pipeline: correlation id, request logging, and
// the inference handler. Lambda and ServeHTTP are its two adapters.
type Service struct {
	handle Handler
}

// New validates settings and builds the pipeline.
func New(settings Settings) (*Service, error) {
	if settings.MaxBodyBytes <= 0 {
		return nil, errors.New("MaxBodyBytes must be positive")
	}
	if err := settings.Limits.Validate(); err != nil {
		return nil, err
	}
	return &Service{handle: Chain(inferHandler(settings), CorrelationID(), RequestLogging())}, nil
}

// Serve runs one request through the pipeline. Adapters and tests share
// this entry point.
func (s *Service) Serve(ctx context.Context, request Request) Response {
	return s.handle(ctx, request)
}
