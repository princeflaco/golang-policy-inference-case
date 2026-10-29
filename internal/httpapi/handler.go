package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/princeflaco/golang-policy-inference-case/internal/apperr"
	"github.com/princeflaco/golang-policy-inference-case/internal/observability"
	"github.com/princeflaco/golang-policy-inference-case/internal/policy"
)

const (
	inferPath         = "/infer"
	headerContentType = "content-type"
	jsonMediaType     = "application/json"
	fieldPolicyDot    = "policy_dot"
	fieldInput        = "input"
)

// inferRequest documents the POST /infer wire contract for OpenAPI
// generation; decodeRequest validates the same fields manually so that
// duplicate keys and unknown fields are rejected.
type inferRequest struct {
	PolicyDot string         `json:"policy_dot" example:"digraph P { start [result=\"\"]; ok [result=\"approved=true\"]; start -> ok [cond=\"age>=18\"] }"`
	Input     map[string]any `json:"input" swaggertype:"object"`
}

// inferResponse is the success envelope of POST /infer.
type inferResponse struct {
	Output map[string]any `json:"output" swaggertype:"object"`
}

// statusByCode is the HTTP mapping of the wire contract's error codes.
var statusByCode = map[apperr.Code]int{
	apperr.InvalidJSON:      http.StatusBadRequest,
	apperr.InvalidInput:     http.StatusBadRequest,
	apperr.InvalidPolicy:    http.StatusBadRequest,
	apperr.InferenceError:   http.StatusBadRequest,
	apperr.TooLarge:         http.StatusRequestEntityTooLarge,
	apperr.MethodNotAllowed: http.StatusMethodNotAllowed,
	apperr.NotFound:         http.StatusNotFound,
	apperr.Internal:         http.StatusInternalServerError,
}

// inferHandler validates the request and runs one policy inference. It
// intentionally contains no policy evaluation rules.
//
//	@Summary		Run one deterministic inference over a DOT policy
//	@Description	Validates the request, parses the DOT policy graph, executes a single path, and returns the enriched variables.
//	@Tags			inference
//	@Accept			json
//	@Produce		json
//	@Param			request	body		inferRequest	true	"DOT policy and input variables"
//	@Success		200		{object}	inferResponse	"input variables plus inferred values"
//	@Failure		400		{object}	apperr.Error	"invalid_json, invalid_input, invalid_policy or inference_error"
//	@Failure		404		{object}	apperr.Error	"path is not /infer"
//	@Failure		405		{object}	apperr.Error	"method is not POST"
//	@Failure		413		{object}	apperr.Error	"request body exceeds the configured limit"
//	@Failure		500		{object}	apperr.Error	"unexpected internal failure"
//	@Router			/infer [post]
func inferHandler(settings Settings) Handler {
	return func(ctx context.Context, request Request) Response {
		if request.Method != http.MethodPost {
			return fail(ctx, apperr.New(apperr.MethodNotAllowed, "method must be POST"))
		}
		if request.Path != inferPath {
			return fail(ctx, apperr.New(apperr.NotFound, "path must be /infer"))
		}
		if !isJSONContentType(request.Headers) {
			return fail(ctx, apperr.New(apperr.InvalidJSON, "Content-Type must be application/json"))
		}

		body, err := readBody(request.Body, settings.MaxBodyBytes)
		if err != nil {
			return fail(ctx, err)
		}

		policyDOT, input, err := decodeRequest(body)
		if err != nil {
			return fail(ctx, err)
		}

		_, parseSpan := observability.StartSpan(ctx, "policy.parse")
		parsed, err := policy.Parse(policyDOT, settings.Limits)
		parseSpan.End()
		if err != nil {
			return fail(ctx, err)
		}
		observability.Logger(ctx).Info("policy parsed", "nodes", parsed.NodeCount(), "edges", parsed.EdgeCount())

		_, inferSpan := observability.StartSpan(ctx, "policy.infer")
		output, err := parsed.Infer(input)
		inferSpan.End()
		if err != nil {
			return fail(ctx, err)
		}
		observability.Logger(ctx).Info("inference completed", "input_vars", len(input), "output_vars", len(output))

		payload, err := json.Marshal(inferResponse{Output: output})
		if err != nil {
			return fail(ctx, apperr.New(apperr.Internal, "failed to encode response"))
		}
		return jsonResponse(http.StatusOK, payload)
	}
}

// readBody reads at most limit+1 bytes, so memory stays bounded while an
// oversized body is still detected.
func readBody(body io.Reader, limit int) ([]byte, error) {
	if body == nil {
		body = bytes.NewReader(nil)
	}
	data, err := io.ReadAll(io.LimitReader(body, int64(limit)+1))
	if err != nil {
		if apperr.From(err) == nil {
			err = apperr.New(apperr.InvalidJSON, "request body could not be read")
		}
		return nil, err
	}
	if len(data) > limit {
		return nil, apperr.New(apperr.TooLarge, "request body exceeds the configured limit")
	}
	return data, nil
}

// fail converts a coded error into the wire response. Uncoded errors are
// unexpected: they are logged and collapsed into internal_error so
// implementation details never leak to the client.
func fail(ctx context.Context, err error) Response {
	apiErr := apperr.From(err)
	if apiErr == nil {
		observability.Logger(ctx).Error("unexpected error", "error", err)
		apiErr = &apperr.Error{Code: apperr.Internal, Message: "internal error"}
	}
	status, ok := statusByCode[apiErr.Code]
	if !ok {
		status = http.StatusInternalServerError
	}
	payload, _ := json.Marshal(apiErr)
	response := jsonResponse(status, payload)
	response.Code = apiErr.Code
	return response
}

func isJSONContentType(headers map[string]string) bool {
	mediaType := strings.TrimSpace(strings.Split(headers[headerContentType], ";")[0])
	return strings.EqualFold(mediaType, jsonMediaType)
}

func jsonResponse(status int, payload []byte) Response {
	return Response{
		Status:  status,
		Headers: map[string]string{"Content-Type": jsonMediaType},
		Body:    payload,
	}
}
