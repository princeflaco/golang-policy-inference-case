package httpapi

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/princeflaco/golang-policy-inference-case/internal/apperr"
)

// Lambda adapts the service to API Gateway HTTP API (payload v2.0) events.
// It owns the gateway's quirks: the stage prefix on RawPath, base64 bodies,
// and the gateway request id used as the correlation fallback.
func (s *Service) Lambda(ctx context.Context, event events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	path := event.RawPath
	if stage := event.RequestContext.Stage; stage != "" {
		path = strings.TrimPrefix(path, "/"+stage)
	}
	var body io.Reader = strings.NewReader(event.Body)
	if event.IsBase64Encoded {
		body = base64Body{base64.NewDecoder(base64.StdEncoding, body)}
	}
	response := s.Serve(ctx, Request{
		Method:    event.RequestContext.HTTP.Method,
		Path:      path,
		Headers:   lowerKeys(event.Headers),
		Body:      body,
		RequestID: event.RequestContext.RequestID,
	})
	return events.APIGatewayV2HTTPResponse{
		StatusCode: response.Status,
		Headers:    response.Headers,
		Body:       string(response.Body),
	}, nil
}

// base64Body decodes lazily so a malformed body surfaces inside the
// pipeline, with correlation id and logging, as invalid_json.
type base64Body struct{ r io.Reader }

func (b base64Body) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		err = apperr.New(apperr.InvalidJSON, "request body is not valid base64")
	}
	return n, err
}

func lowerKeys(headers map[string]string) map[string]string {
	lowered := make(map[string]string, len(headers))
	for key, value := range headers {
		lowered[strings.ToLower(key)] = value
	}
	return lowered
}
