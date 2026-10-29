package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
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
