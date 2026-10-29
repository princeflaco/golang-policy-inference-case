package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/princeflaco/golang-policy-inference-case/internal/apperr"
	"github.com/princeflaco/golang-policy-inference-case/internal/observability"
	"github.com/princeflaco/golang-policy-inference-case/internal/policy"
)

const testPolicy = `digraph Policy {
	start [result=""]
	approved [result="approved=true"]
	rejected [result="approved=false"]
	start -> approved [cond="age>=18"]
	start -> rejected [cond="age<18"]
}`

func defaultSettings() Settings {
	return Settings{MaxBodyBytes: 1 << 20, Limits: policy.DefaultLimits()}
}

func newService(t testing.TB, settings Settings) *Service {
	t.Helper()
	service, err := New(settings)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
}

func request(body string) Request {
	return Request{
		Method:  "POST",
		Path:    "/infer",
		Headers: map[string]string{"content-type": "application/json"},
		Body:    strings.NewReader(body),
	}
}

// payload builds a request body with the policy and input JSON-encoded.
func payload(t testing.TB, dot string, input any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"policy_dot": dot, "input": input})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func serve(t testing.TB, req Request) Response {
	t.Helper()
	return newService(t, defaultSettings()).Serve(context.Background(), req)
}

func output(t testing.TB, response Response) map[string]any {
	t.Helper()
	if response.Status != 200 {
		t.Fatalf("Status = %d, body = %s", response.Status, response.Body)
	}
	var body inferResponse
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	return body.Output
}

func assertError(t testing.TB, response Response, status int, code apperr.Code) {
	t.Helper()
	if response.Status != status {
		t.Fatalf("Status = %d, want %d, body = %s", response.Status, status, response.Body)
	}
	var body apperr.Error
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatalf("error body is not JSON: %v (%s)", err, response.Body)
	}
	if body.Code != code || response.Code != code {
		t.Fatalf("body code = %q, response code = %q, want %q", body.Code, response.Code, code)
	}
	if body.Message == "" {
		t.Fatal("error body has no message")
	}
	if strings.Contains(string(response.Body), "output") {
		t.Fatalf("error body leaked output: %s", response.Body)
	}
}

func TestServeInfersBothOutcomes(t *testing.T) {
	for age, approved := range map[float64]bool{20: true, 16: false} {
		got := output(t, serve(t, request(payload(t, testPolicy, map[string]any{"age": age}))))
		if got["approved"] != approved || got["age"] != age {
			t.Fatalf("age %v: output = %#v", age, got)
		}
	}
}

func TestServeInfersStringAndBooleanValues(t *testing.T) {
	dot := `digraph P {
		start [result="segment=prime,score=720"]
		approved [result="approved=true"]
		rejected [result="approved=false"]
		start -> approved [cond="segment==\"prime\""]
		start -> rejected [cond="active==false"]
	}`
	got := output(t, serve(t, request(payload(t, dot, map[string]any{"active": true}))))
	if got["segment"] != "prime" || got["score"] != 720.0 || got["approved"] != true {
		t.Fatalf("output = %#v", got)
	}
}

func TestServeInfersThroughIntermediateNode(t *testing.T) {
	dot := `digraph P {
		start [result="age=17"]
		check [result="age=20"]
		approved [result="approved=true"]
		start -> check [cond="age==17"]
		check -> approved [cond="age>=18"]
	}`
	got := output(t, serve(t, request(payload(t, dot, map[string]any{}))))
	if got["age"] != 20.0 || got["approved"] != true {
		t.Fatalf("output = %#v", got)
	}
}

func TestServeInfersCompoundReadmePolicy(t *testing.T) {
	dot := `digraph Policy {
		start [result=""]
		approved [result="approved=true"]
		review [result="approved=false,status=\"review\""]
		rejected [result="approved=false"]
		start -> approved [cond="age>=18 && score>700"]
		start -> review [cond="age>=18 && score<=700"]
		start -> rejected [cond="age<18"]
	}`
	got := output(t, serve(t, request(payload(t, dot, map[string]any{"age": 20, "score": 720}))))
	if got["approved"] != true || got["score"] != 720.0 {
		t.Fatalf("output = %#v", got)
	}
}

func TestServeRejectsContractViolations(t *testing.T) {
	valid := payload(t, testPolicy, map[string]any{"age": 20})
	policyJSON, _ := json.Marshal(testPolicy)
	withInput := func(input string) string { return `{"policy_dot":` + string(policyJSON) + `,"input":` + input + `}` }
	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		status      int
		code        apperr.Code
	}{
		{"method", "GET", "/infer", "application/json", valid, 405, apperr.MethodNotAllowed},
		{"path", "POST", "/other", "application/json", valid, 404, apperr.NotFound},
		{"content type", "POST", "/infer", "text/plain", valid, 400, apperr.InvalidJSON},
		{"missing content type", "POST", "/infer", "", valid, 400, apperr.InvalidJSON},
		{"malformed json", "POST", "/infer", "application/json", `{`, 400, apperr.InvalidJSON},
		{"empty body", "POST", "/infer", "application/json", ``, 400, apperr.InvalidJSON},
		{"top-level array", "POST", "/infer", "application/json", `[1,{"a":1}]`, 400, apperr.InvalidJSON},
		{"trailing value", "POST", "/infer", "application/json", valid + `{}`, 400, apperr.InvalidJSON},
		{"duplicate key", "POST", "/infer", "application/json", `{"policy_dot":"a","policy_dot":"b","input":{}}`, 400, apperr.InvalidJSON},
		{"nested duplicate key", "POST", "/infer", "application/json", withInput(`{"age":1,"age":2}`), 400, apperr.InvalidJSON},
		{"extra field", "POST", "/infer", "application/json", `{"policy_dot":` + string(policyJSON) + `,"input":{},"extra":true}`, 400, apperr.InvalidJSON},
		{"missing policy", "POST", "/infer", "application/json", `{"input":{}}`, 400, apperr.InvalidJSON},
		{"policy type", "POST", "/infer", "application/json", `{"policy_dot":123,"input":{}}`, 400, apperr.InvalidJSON},
		{"blank policy", "POST", "/infer", "application/json", `{"policy_dot":"  ","input":{}}`, 400, apperr.InvalidJSON},
		{"missing input", "POST", "/infer", "application/json", `{"policy_dot":` + string(policyJSON) + `}`, 400, apperr.InvalidInput},
		{"input not object", "POST", "/infer", "application/json", withInput(`[1]`), 400, apperr.InvalidInput},
		{"null input", "POST", "/infer", "application/json", withInput(`null`), 400, apperr.InvalidInput},
		{"non-finite number", "POST", "/infer", "application/json", withInput(`{"age":1e999}`), 400, apperr.InvalidInput},
		{"object value", "POST", "/infer", "application/json", withInput(`{"age":{"value":20}}`), 400, apperr.InvalidInput},
		{"list value", "POST", "/infer", "application/json", withInput(`{"age":[20]}`), 400, apperr.InvalidInput},
		{"null value", "POST", "/infer", "application/json", withInput(`{"age":null}`), 400, apperr.InvalidInput},
		{"bad key", "POST", "/infer", "application/json", withInput(`{"age":"20","bad-key":true}`), 400, apperr.InvalidInput},
		{"invalid policy", "POST", "/infer", "application/json", payload(t, `digraph P { start -> ok [cond="age+1>18"] }`, map[string]any{}), 400, apperr.InvalidPolicy},
		{"type mismatch", "POST", "/infer", "application/json", payload(t, `digraph P { start -> ok [cond="age>=\"18\""] }`, map[string]any{"age": 20}), 400, apperr.InferenceError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := request(test.body)
			req.Method, req.Path = test.method, test.path
			req.Headers["content-type"] = test.contentType
			assertError(t, serve(t, req), test.status, test.code)
		})
	}
}

func TestServeAcceptsContentTypeParameters(t *testing.T) {
	req := request(payload(t, testPolicy, map[string]any{"age": 20}))
	req.Headers["content-type"] = "Application/JSON; charset=utf-8"
	output(t, serve(t, req))
}

func TestServeEnforcesBodyLimit(t *testing.T) {
	body := payload(t, testPolicy, map[string]any{"age": 20})
	settings := defaultSettings()
	settings.MaxBodyBytes = len(body)
	output(t, newService(t, settings).Serve(context.Background(), request(body)))

	settings.MaxBodyBytes = len(body) - 1
	assertError(t, newService(t, settings).Serve(context.Background(), request(body)), 413, apperr.TooLarge)
}

func TestServeAppliesPolicyLimits(t *testing.T) {
	settings := defaultSettings()
	settings.Limits.MaxPolicyNodes = 1
	response := newService(t, settings).Serve(context.Background(), request(payload(t, testPolicy, map[string]any{"age": 20})))
	assertError(t, response, 400, apperr.InvalidPolicy)
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestServeReportsBodyReadFailures(t *testing.T) {
	req := request("")
	req.Body = failingReader{errors.New("connection reset")}
	assertError(t, serve(t, req), 400, apperr.InvalidJSON)

	req.Body = failingReader{apperr.New(apperr.TooLarge, "coded by the adapter")}
	assertError(t, serve(t, req), 413, apperr.TooLarge)
}

func TestServeTreatsMissingBodyAsEmpty(t *testing.T) {
	req := request("")
	req.Body = nil
	assertError(t, serve(t, req), 400, apperr.InvalidJSON)
}

func TestFailCollapsesUncodedErrors(t *testing.T) {
	var logs bytes.Buffer
	ctx := observability.WithLogger(context.Background(), slog.New(slog.NewTextHandler(&logs, nil)))
	response := fail(ctx, errors.New("secret implementation detail"))
	assertError(t, response, 500, apperr.Internal)
	if strings.Contains(string(response.Body), "secret") {
		t.Fatalf("internal detail leaked to client: %s", response.Body)
	}
	if !strings.Contains(logs.String(), "secret implementation detail") {
		t.Fatalf("uncoded error was not logged: %q", logs.String())
	}
}

func TestFailMapsUnknownCodesTo500(t *testing.T) {
	assertError(t, fail(context.Background(), apperr.New("brand_new_code", "x")), 500, "brand_new_code")
}

func TestNewRejectsInvalidSettings(t *testing.T) {
	bodyless := defaultSettings()
	bodyless.MaxBodyBytes = 0
	unlimited := defaultSettings()
	unlimited.Limits.MaxPolicyEdges = 0
	for name, settings := range map[string]Settings{"body": bodyless, "limits": unlimited} {
		if _, err := New(settings); err == nil {
			t.Errorf("New() with invalid %s settings unexpectedly succeeded", name)
		}
	}
}

func TestServeLogsCodesWithoutExposingPolicyOrInput(t *testing.T) {
	var logs bytes.Buffer
	ctx := observability.WithLogger(context.Background(), slog.New(slog.NewTextHandler(&logs, nil)))
	service := newService(t, defaultSettings())

	ok := service.Serve(ctx, request(payload(t, `digraph P { start [result="secret=\"super-secret-value\""] }`, map[string]any{"secret_input": "super-secret-value"})))
	output(t, ok)
	bad := service.Serve(ctx, request(payload(t, `digraph P { start -> x [cond="1+1"] }`, map[string]any{})))
	assertError(t, bad, 400, apperr.InvalidPolicy)

	text := logs.String()
	for _, want := range []string{"status=200", "status=400", "code=invalid_policy", "duration_ms=", "span=policy.parse"} {
		if !strings.Contains(text, want) {
			t.Fatalf("logs missing %q: %q", want, text)
		}
	}
	if strings.Contains(text, "super-secret-value") || strings.Contains(text, "secret_input") {
		t.Fatalf("log leaked policy or input: %q", text)
	}
}
