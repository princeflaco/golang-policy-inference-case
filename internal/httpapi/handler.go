package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/princeflaco/golang-policy-inference-case/internal/apperr"
	"github.com/princeflaco/golang-policy-inference-case/internal/config"
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

// Handle is the request pipeline: correlation id, request logging, then the
// inference handler itself.
var Handle = Chain(core, CorrelationID(), RequestLogging())

// core translates an API Gateway HTTP API v2 request into a policy
// inference. It intentionally contains no policy evaluation rules.
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
func core(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	cfg, err := config.Load()
	if err != nil {
		observability.Logger(ctx).Error("invalid configuration", "error", err)
		return fail(ctx, apperr.New(apperr.Internal, "invalid configuration"))
	}

	if request.RequestContext.HTTP.Method != http.MethodPost {
		return fail(ctx, apperr.New(apperr.MethodNotAllowed, "method must be POST"))
	}
	path := request.RawPath
	if stage := request.RequestContext.Stage; stage != "" {
		path = strings.TrimPrefix(path, "/"+stage)
	}
	if path != inferPath {
		return fail(ctx, apperr.New(apperr.NotFound, "path must be /infer"))
	}
	if !isJSONContentType(request.Headers) {
		return fail(ctx, apperr.New(apperr.InvalidJSON, "Content-Type must be application/json"))
	}

	body, err := requestBody(request)
	if err != nil {
		return fail(ctx, err)
	}
	if len(body) > cfg.MaxBodyBytes {
		return fail(ctx, apperr.New(apperr.TooLarge, "request body exceeds the configured limit"))
	}

	policyDOT, input, err := decodeRequest(body)
	if err != nil {
		return fail(ctx, err)
	}

	parseCtx, parseSpan := observability.StartSpan(ctx, "policy.parse")
	parsed, err := policy.Parse(policyDOT, cfg.Limits())
	parseSpan.End(parseCtx)
	if err != nil {
		return fail(ctx, err)
	}
	observability.Logger(ctx).Info("policy parsed", "nodes", parsed.NodeCount(), "edges", parsed.EdgeCount())

	inferCtx, inferSpan := observability.StartSpan(ctx, "policy.infer")
	output, err := parsed.Infer(input)
	inferSpan.End(inferCtx)
	if err != nil {
		return fail(ctx, err)
	}
	observability.Logger(ctx).Info("inference completed", "input_vars", len(input), "output_vars", len(output))

	payload, err := json.Marshal(inferResponse{Output: output})
	if err != nil {
		return fail(ctx, apperr.New(apperr.Internal, "failed to encode response"))
	}
	return jsonResponse(http.StatusOK, payload), nil
}

// fail converts a coded error into the wire response. Uncoded errors are
// unexpected: they are logged and collapsed into internal_error so
// implementation details never leak to the client.
func fail(ctx context.Context, err error) (events.APIGatewayV2HTTPResponse, error) {
	apiErr := apperr.From(err)
	if apiErr == nil {
		observability.Logger(ctx).Error("unexpected error", "error", err)
		apiErr = &apperr.Error{Code: apperr.Internal, Message: "internal error"}
	}
	status, ok := statusByCode[apiErr.Code]
	if !ok {
		status = http.StatusInternalServerError
	}
	return errorResponse(status, apiErr), nil
}

func requestBody(request events.APIGatewayV2HTTPRequest) ([]byte, error) {
	if !request.IsBase64Encoded {
		return []byte(request.Body), nil
	}
	body, err := base64.StdEncoding.DecodeString(request.Body)
	if err != nil {
		return nil, apperr.New(apperr.InvalidJSON, "request body is not valid base64")
	}
	return body, nil
}

func isJSONContentType(headers map[string]string) bool {
	for key, value := range headers {
		if strings.EqualFold(key, headerContentType) {
			mediaType := strings.TrimSpace(strings.Split(value, ";")[0])
			return strings.EqualFold(mediaType, jsonMediaType)
		}
	}
	return false
}

func decodeRequest(body []byte) (string, map[string]any, error) {
	if err := rejectDuplicateKeys(body); err != nil {
		return "", nil, apperr.New(apperr.InvalidJSON, "%v", err)
	}

	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&raw); err != nil {
		return "", nil, apperr.New(apperr.InvalidJSON, "request body must be a JSON object")
	}
	if decoder.More() {
		return "", nil, apperr.New(apperr.InvalidJSON, "request body must contain one JSON value")
	}
	if _, ok := raw[fieldPolicyDot]; !ok {
		return "", nil, apperr.New(apperr.InvalidJSON, "policy_dot is required")
	}
	if _, ok := raw[fieldInput]; !ok {
		return "", nil, apperr.New(apperr.InvalidInput, "input is required")
	}
	if len(raw) != 2 {
		return "", nil, apperr.New(apperr.InvalidJSON, "request must contain exactly policy_dot and input")
	}

	var policyDOT string
	if err := json.Unmarshal(raw[fieldPolicyDot], &policyDOT); err != nil || strings.TrimSpace(policyDOT) == "" {
		return "", nil, apperr.New(apperr.InvalidJSON, "policy_dot must be a non-empty string")
	}

	var rawInput map[string]json.RawMessage
	if err := json.Unmarshal(raw[fieldInput], &rawInput); err != nil || rawInput == nil {
		return "", nil, apperr.New(apperr.InvalidInput, "input must be a JSON object")
	}

	input := make(map[string]any, len(rawInput))
	for key, rawValue := range rawInput {
		if !policy.IsIdentifier(key) {
			return "", nil, apperr.New(apperr.InvalidInput, "input key %q is not a valid identifier", key)
		}
		value, err := decodeInputValue(rawValue)
		if err != nil {
			return "", nil, apperr.New(apperr.InvalidInput, "input.%s: %v", key, err)
		}
		input[key] = value
	}
	return policyDOT, input, nil
}

func decodeInputValue(raw json.RawMessage) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("invalid JSON value")
	}
	switch typed := value.(type) {
	case string, bool:
		return typed, nil
	case json.Number:
		number, err := strconv.ParseFloat(typed.String(), 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return nil, errors.New("number must be finite")
		}
		return number, nil
	default:
		return nil, errors.New("value must be a string, number, or boolean")
	}
}

// rejectDuplicateKeys walks the JSON syntax without relying on map decoding,
// because map decoding silently keeps the last duplicate key.
func rejectDuplicateKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := walkJSONValue(decoder); err != nil {
		return err
	}
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("request must contain one JSON value")
}

func walkJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key must be a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = true
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	default:
		return errors.New("invalid JSON delimiter")
	}
}

func errorResponse(status int, apiErr *apperr.Error) events.APIGatewayV2HTTPResponse {
	payload, _ := json.Marshal(apiErr)
	return jsonResponse(status, payload)
}

func jsonResponse(status int, payload []byte) events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(payload),
	}
}
