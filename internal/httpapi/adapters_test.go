package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/princeflaco/golang-policy-inference-case/internal/apperr"
)

func lambdaEvent(body string) events.APIGatewayV2HTTPRequest {
	return events.APIGatewayV2HTTPRequest{
		RawPath: "/infer",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    body,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "POST"},
		},
	}
}

func invokeLambda(t *testing.T, event events.APIGatewayV2HTTPRequest) Response {
	t.Helper()
	response, err := newService(t, defaultSettings()).Lambda(context.Background(), event)
	if err != nil {
		t.Fatalf("Lambda() error = %v", err)
	}
	return Response{Status: response.StatusCode, Headers: response.Headers, Body: []byte(response.Body), Code: decodeCode(response.Body)}
}

func decodeCode(body string) apperr.Code {
	var apiErr apperr.Error
	_ = json.Unmarshal([]byte(body), &apiErr)
	return apiErr.Code
}

func TestLambdaAdapter(t *testing.T) {
	body := payload(t, testPolicy, map[string]any{"age": 20})

	t.Run("plain body with mixed-case headers", func(t *testing.T) {
		response := invokeLambda(t, lambdaEvent(body))
		if output(t, response)["approved"] != true || response.Headers["Content-Type"] != "application/json" {
			t.Fatalf("response = %+v", response)
		}
	})

	t.Run("strips the stage prefix", func(t *testing.T) {
		event := lambdaEvent(body)
		event.RawPath, event.RequestContext.Stage = "/dev/infer", "dev"
		output(t, invokeLambda(t, event))
	})

	t.Run("decodes base64 bodies", func(t *testing.T) {
		event := lambdaEvent(base64.StdEncoding.EncodeToString([]byte(body)))
		event.IsBase64Encoded = true
		output(t, invokeLambda(t, event))
	})

	t.Run("invalid base64 stays inside the contract", func(t *testing.T) {
		event := lambdaEvent("%%% not base64 %%%")
		event.IsBase64Encoded = true
		response := invokeLambda(t, event)
		assertError(t, response, 400, apperr.InvalidJSON)
		if response.Headers["x-correlation-id"] == "" {
			t.Fatal("error response lost x-correlation-id")
		}
	})

	t.Run("gateway request id becomes the correlation id", func(t *testing.T) {
		event := lambdaEvent(body)
		event.RequestContext.RequestID = "gw-req-1"
		if got := invokeLambda(t, event).Headers["x-correlation-id"]; got != "gw-req-1" {
			t.Fatalf("x-correlation-id = %q", got)
		}
	})
}

func TestHTTPAdapter(t *testing.T) {
	body := payload(t, testPolicy, map[string]any{"age": 20})
	settings := defaultSettings()
	settings.MaxBodyBytes = len(body)
	server := httptest.NewServer(newService(t, settings))
	t.Cleanup(server.Close)

	post := func(t *testing.T, path, body string, headers map[string]string) (*http.Response, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res, data
	}
	jsonHeaders := map[string]string{"Content-Type": "application/json", "X-Correlation-Id": "http-corr"}

	t.Run("success", func(t *testing.T) {
		res, data := post(t, "/infer", body, jsonHeaders)
		if output(t, Response{Status: res.StatusCode, Body: data})["approved"] != true {
			t.Fatalf("body = %s", data)
		}
		if res.Header.Get("X-Correlation-Id") != "http-corr" {
			t.Fatalf("correlation header = %q", res.Header.Get("X-Correlation-Id"))
		}
	})

	t.Run("oversized body is a JSON 413", func(t *testing.T) {
		res, data := post(t, "/infer", body+" ", jsonHeaders)
		if res.Header.Get("Content-Type") != "application/json" || res.Header.Get("X-Correlation-Id") != "http-corr" {
			t.Fatalf("headers = %v", res.Header)
		}
		assertError(t, Response{Status: res.StatusCode, Body: data, Code: decodeCode(string(data))}, 413, apperr.TooLarge)
	})

	t.Run("unknown path", func(t *testing.T) {
		res, data := post(t, "/nope", body, jsonHeaders)
		assertError(t, Response{Status: res.StatusCode, Body: data, Code: decodeCode(string(data))}, 404, apperr.NotFound)
	})
}
