package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/princeflaco/golang-policy-inference-case/internal/apperr"
)

// decodeRequest reads the POST /infer body in a single tokenization pass,
// rejecting duplicate keys at any depth (map decoding would silently keep
// the last one), trailing values, and unknown top-level fields.
//
// It owns the JSON shape only. The input contract (identifier grammar and
// scalar value types) belongs to policy.Infer; decodeRequest just converts
// JSON numbers to float64, which is the representation the engine accepts.
func decodeRequest(body []byte) (string, map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	value, err := decodeJSONValue(decoder)
	if err != nil {
		return "", nil, apperr.New(apperr.InvalidJSON, "%v", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return "", nil, apperr.New(apperr.InvalidJSON, "request body must contain one JSON value")
	}

	raw, ok := value.(map[string]any)
	if !ok {
		return "", nil, apperr.New(apperr.InvalidJSON, "request body must be a JSON object")
	}
	policyValue, ok := raw[fieldPolicyDot]
	if !ok {
		return "", nil, apperr.New(apperr.InvalidJSON, "policy_dot is required")
	}
	inputValue, ok := raw[fieldInput]
	if !ok {
		return "", nil, apperr.New(apperr.InvalidInput, "input is required")
	}
	if len(raw) != 2 {
		return "", nil, apperr.New(apperr.InvalidJSON, "request must contain exactly policy_dot and input")
	}

	policyDOT, ok := policyValue.(string)
	if !ok || strings.TrimSpace(policyDOT) == "" {
		return "", nil, apperr.New(apperr.InvalidJSON, "policy_dot must be a non-empty string")
	}
	rawInput, ok := inputValue.(map[string]any)
	if !ok {
		return "", nil, apperr.New(apperr.InvalidInput, "input must be a JSON object")
	}

	input := make(map[string]any, len(rawInput))
	for key, value := range rawInput {
		if number, ok := value.(json.Number); ok {
			parsed, err := strconv.ParseFloat(number.String(), 64)
			if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
				return "", nil, apperr.New(apperr.InvalidInput, "input.%s: number must be finite", key)
			}
			value = parsed
		}
		input[key] = value
	}
	return policyDOT, input, nil
}

// decodeJSONValue builds one JSON value from the token stream: objects become
// map[string]any, arrays []any, numbers json.Number.
func decodeJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("object key must be a string")
			}
			if _, duplicate := object[key]; duplicate {
				return nil, fmt.Errorf("duplicate JSON key %q", key)
			}
			if object[key], err = decodeJSONValue(decoder); err != nil {
				return nil, err
			}
		}
		_, err := decoder.Token()
		return object, err
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			element, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, element)
		}
		_, err := decoder.Token()
		return array, err
	default:
		return nil, errors.New("invalid JSON delimiter")
	}
}
