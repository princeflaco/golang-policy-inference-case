package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/princeflaco/golang-policy-inference-case/internal/policy"
)

const (
	defaultMaxBodyBytes = 1048576
	codeInvalidJSON     = "invalid_json"
	codeInvalidInput    = "invalid_input"
	codeInvalidPolicy   = "invalid_policy"
	codeInferenceError  = "inference_error"
	codeTooLarge        = "request_too_large"
	codeMethod          = "method_not_allowed"
	codeNotFound        = "not_found"
	codeInternal        = "internal_error"
)

var inputIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Handle translates an API Gateway HTTP API v2 request into a policy
// inference. It intentionally contains no policy evaluation rules.
func Handle(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	_ = ctx
	started := time.Now()
	respond := func(response events.APIGatewayV2HTTPResponse, code string) (events.APIGatewayV2HTTPResponse, error) {
		slog.Info("infer request completed", "status", response.StatusCode, "code", code, "duration_ms", time.Since(started).Milliseconds())
		return response, nil
	}

	if request.RequestContext.HTTP.Method != "POST" {
		return respond(errorResponse(405, codeMethod, "method must be POST"), codeMethod)
	}
	if request.RawPath != "/infer" {
		return respond(errorResponse(404, codeNotFound, "path must be /infer"), codeNotFound)
	}
	if !isJSONContentType(request.Headers) {
		return respond(errorResponse(400, codeInvalidJSON, "Content-Type must be application/json"), codeInvalidJSON)
	}

	body, err := requestBody(request)
	if err != nil {
		return respond(errorResponse(400, codeInvalidJSON, err.Error()), codeInvalidJSON)
	}
	if len(body) > maxBodyBytes() {
		return respond(errorResponse(413, codeTooLarge, "request body exceeds the configured limit"), codeTooLarge)
	}

	policyDOT, input, err := decodeRequest(body)
	if err != nil {
		return respond(errorResponse(400, errorCode(err), err.Error()), errorCode(err))
	}

	parsed, err := policy.Parse(policyDOT)
	if err != nil {
		return respond(errorResponse(400, codeInvalidPolicy, publicMessage(err)), codeInvalidPolicy)
	}
	output, err := parsed.Infer(input)
	if err != nil {
		return respond(errorResponse(400, codeInferenceError, publicMessage(err)), codeInferenceError)
	}

	payload, err := json.Marshal(map[string]any{"output": output})
	if err != nil {
		return respond(errorResponse(500, codeInternal, "failed to encode response"), codeInternal)
	}
	return respond(jsonResponse(200, payload), "success")
}

func requestBody(request events.APIGatewayV2HTTPRequest) ([]byte, error) {
	if !request.IsBase64Encoded {
		return []byte(request.Body), nil
	}
	body, err := base64.StdEncoding.DecodeString(request.Body)
	if err != nil {
		return nil, errors.New("request body is not valid base64")
	}
	return body, nil
}

func isJSONContentType(headers map[string]string) bool {
	for key, value := range headers {
		if strings.EqualFold(key, "content-type") {
			mediaType := strings.TrimSpace(strings.Split(value, ";")[0])
			return strings.EqualFold(mediaType, "application/json")
		}
	}
	return false
}

func decodeRequest(body []byte) (string, map[string]any, error) {
	if err := rejectDuplicateKeys(body); err != nil {
		return "", nil, requestError(codeInvalidJSON, err.Error())
	}

	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return "", nil, requestError(codeInvalidJSON, "request body must be a JSON object")
	}
	if decoder.More() {
		return "", nil, requestError(codeInvalidJSON, "request body must contain one JSON value")
	}
	if len(raw) != 2 {
		return "", nil, requestError(codeInvalidJSON, "request must contain exactly policy_dot and input")
	}
	if _, ok := raw["policy_dot"]; !ok {
		return "", nil, requestError(codeInvalidJSON, "policy_dot is required")
	}
	if _, ok := raw["input"]; !ok {
		return "", nil, requestError(codeInvalidJSON, "input is required")
	}

	var policyDOT string
	if err := json.Unmarshal(raw["policy_dot"], &policyDOT); err != nil || strings.TrimSpace(policyDOT) == "" {
		return "", nil, requestError(codeInvalidJSON, "policy_dot must be a non-empty string")
	}

	var rawInput map[string]json.RawMessage
	if err := json.Unmarshal(raw["input"], &rawInput); err != nil || rawInput == nil {
		return "", nil, requestError(codeInvalidInput, "input must be a JSON object")
	}

	input := make(map[string]any, len(rawInput))
	for key, rawValue := range rawInput {
		if !inputIdentifierPattern.MatchString(key) {
			return "", nil, requestError(codeInvalidInput, fmt.Sprintf("input key %q is not a valid identifier", key))
		}
		value, err := decodeInputValue(rawValue)
		if err != nil {
			return "", nil, requestError(codeInvalidInput, fmt.Sprintf("input.%s: %v", key, err))
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

type requestErr struct {
	code    string
	message string
}

func (e *requestErr) Error() string { return e.message }

func requestError(code, message string) error {
	return &requestErr{code: code, message: message}
}

func errorCode(err error) string {
	var requestErr *requestErr
	if errors.As(err, &requestErr) {
		return requestErr.code
	}
	return codeInvalidJSON
}

func publicMessage(err error) string {
	var policyErr *policy.Error
	if errors.As(err, &policyErr) {
		return policyErr.Message
	}
	return codeInternal
}

func maxBodyBytes() int {
	value, err := strconv.Atoi(os.Getenv("MAX_BODY_BYTES"))
	if err != nil || value <= 0 {
		return defaultMaxBodyBytes
	}
	return value
}

func errorResponse(status int, code, message string) events.APIGatewayV2HTTPResponse {
	payload, _ := json.Marshal(errorBody{Code: code, Message: message})
	return jsonResponse(status, payload)
}

func jsonResponse(status int, payload []byte) events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(payload),
	}
}
