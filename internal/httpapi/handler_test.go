package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

const testPolicy = `digraph Policy {
	start [result=""]
	approved [result="approved=true"]
	rejected [result="approved=false"]
	start -> approved [cond="age>=18"]
	start -> rejected [cond="age<18"]
}`

func request(body string) events.APIGatewayV2HTTPRequest {
	return events.APIGatewayV2HTTPRequest{
		RawPath: "/infer",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    body,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "POST"},
		},
	}
}

func TestHandleInfersApproved(t *testing.T) {
	payload := `{"policy_dot":"digraph Policy { start [result=\"\"]; approved [result=\"approved=true\"]; rejected [result=\"approved=false\"]; start -> approved [cond=\"age>=18\"]; start -> rejected [cond=\"age<18\"]; }","input":{"age":20}}`
	response, err := Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	var body struct {
		Output map[string]any `json:"output"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Output["approved"] != true || body.Output["age"] != 20.0 {
		t.Fatalf("unexpected output: %#v", body.Output)
	}
}

func TestHandleRejectsDuplicateJSONKeys(t *testing.T) {
	payload := `{"policy_dot":"` + testPolicy + `","policy_dot":"` + testPolicy + `","input":{"age":20}}`
	response, err := Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if response.StatusCode != 400 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	var body errorBody
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Code != codeInvalidJSON {
		t.Fatalf("code = %q, want %q", body.Code, codeInvalidJSON)
	}
}

func TestHandleRejectsUnsupportedPolicyFeature(t *testing.T) {
	payload := `{"policy_dot":"digraph P { start -> ok [cond=\"age+1>18\"]; ok [result=\"approved=true\"] }","input":{"age":20,"score":720}}`
	response, err := Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if response.StatusCode != 400 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	var body errorBody
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Code != codeInvalidPolicy {
		t.Fatalf("code = %q, want %q", body.Code, codeInvalidPolicy)
	}
}

func TestHandleInfersRejected(t *testing.T) {
	payload := `{"policy_dot":"digraph Policy { start [result=\"\"]; approved [result=\"approved=true\"]; rejected [result=\"approved=false\"]; start -> approved [cond=\"age>=18\"]; start -> rejected [cond=\"age<18\"]; }","input":{"age":16}}`
	response, err := Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	var body struct {
		Output map[string]any `json:"output"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Output["approved"] != false {
		t.Fatalf("unexpected output: %#v", body.Output)
	}
}

func TestHandleRejectsNullInput(t *testing.T) {
	policyJSON, err := json.Marshal(testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"policy_dot":` + string(policyJSON) + `,"input":null}`
	response, err := Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if response.StatusCode != 400 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	var body errorBody
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Code != codeInvalidInput {
		t.Fatalf("code = %q, want %q", body.Code, codeInvalidInput)
	}
}

func TestHandleInfersStringAndBooleanValues(t *testing.T) {
	policyJSON, err := json.Marshal(`digraph P {
		start [result="segment=prime,score=720"]
		approved [result="approved=true"]
		rejected [result="approved=false"]
		start -> approved [cond="segment==\"prime\""]
		start -> rejected [cond="active==false"]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"policy_dot":` + string(policyJSON) + `,"input":{"active":true}}`
	response, err := Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	var body struct {
		Output map[string]any `json:"output"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Output["segment"] != "prime" || body.Output["score"] != 720.0 || body.Output["approved"] != true {
		t.Fatalf("unexpected output: %#v", body.Output)
	}
}

func TestHandleReturnsInferenceErrorForTypeMismatch(t *testing.T) {
	policyJSON, err := json.Marshal(`digraph P {
		start -> ok [cond="age>=\"18\""]
		ok [result="approved=true"]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"policy_dot":` + string(policyJSON) + `,"input":{"age":20}}`
	response, err := Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if response.StatusCode != 400 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	var body errorBody
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Code != codeInferenceError {
		t.Fatalf("code = %q, want %q", body.Code, codeInferenceError)
	}
}

func TestHandleInfersThroughIntermediateNode(t *testing.T) {
	policyJSON, err := json.Marshal(`digraph P {
		start [result="age=17"]
		check [result="age=20"]
		approved [result="approved=true"]
		start -> check [cond="age==17"]
		check -> approved [cond="age>=18"]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"policy_dot":` + string(policyJSON) + `,"input":{}}`
	response, err := Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	var body struct {
		Output map[string]any `json:"output"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Output["age"] != 20.0 || body.Output["approved"] != true {
		t.Fatalf("unexpected output: %#v", body.Output)
	}
}

func TestHandleInfersCompoundReadmePolicy(t *testing.T) {
	policyJSON, err := json.Marshal(`digraph Policy {
		start [result=""]
		approved [result="approved=true"]
		review [result="approved=false,status=\"review\""]
		rejected [result="approved=false"]
		start -> approved [cond="age>=18 && score>700"]
		start -> review [cond="age>=18 && score<=700"]
		start -> rejected [cond="age<18"]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"policy_dot":` + string(policyJSON) + `,"input":{"age":20,"score":720}}`
	response, err := Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	var body struct {
		Output map[string]any `json:"output"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Output["approved"] != true || body.Output["score"] != 720.0 {
		t.Fatalf("unexpected output: %#v", body.Output)
	}
}

func TestHandleRejectsHTTPContractViolations(t *testing.T) {
	policyJSON, err := json.Marshal(testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	validPayload := `{"policy_dot":` + string(policyJSON) + `,"input":{"age":20}}`
	tests := []struct {
		name       string
		method     string
		path       string
		contentTyp string
		body       string
		status     int
		code       string
	}{
		{"method", "GET", "/infer", "application/json", validPayload, 405, codeMethod},
		{"path", "POST", "/other", "application/json", validPayload, 404, codeNotFound},
		{"content type", "POST", "/infer", "text/plain", validPayload, 400, codeInvalidJSON},
		{"malformed json", "POST", "/infer", "application/json", `{`, 400, codeInvalidJSON},
		{"extra field", "POST", "/infer", "application/json", `{"policy_dot":` + string(policyJSON) + `,"input":{},"extra":true}`, 400, codeInvalidJSON},
		{"missing input", "POST", "/infer", "application/json", `{"policy_dot":` + string(policyJSON) + `}`, 400, codeInvalidJSON},
		{"policy type", "POST", "/infer", "application/json", `{"policy_dot":123,"input":{}}`, 400, codeInvalidJSON},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := request(test.body)
			req.RequestContext.HTTP.Method = test.method
			req.RawPath = test.path
			req.Headers["Content-Type"] = test.contentTyp
			response, err := Handle(context.Background(), req)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if response.StatusCode != test.status {
				t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
			}
			var body errorBody
			if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			if body.Code != test.code {
				t.Fatalf("code = %q, want %q", body.Code, test.code)
			}
		})
	}
}

func TestHandleRejectsUnsupportedInputValues(t *testing.T) {
	policyJSON, err := json.Marshal(testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	tests := []string{
		`{"age":{"value":20}}`,
		`{"age":[20]}`,
		`{"age":null}`,
		`{"age":"20","bad-key":true}`,
	}
	for _, inputJSON := range tests {
		payload := `{"policy_dot":` + string(policyJSON) + `,"input":` + inputJSON + `}`
		response, err := Handle(context.Background(), request(payload))
		if err != nil {
			t.Fatalf("Handle() error = %v", err)
		}
		if response.StatusCode != 400 {
			t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
		}
		var body errorBody
		if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
			t.Fatalf("response is not JSON: %v", err)
		}
		if body.Code != codeInvalidInput {
			t.Fatalf("code = %q, want %q for input %s", body.Code, codeInvalidInput, inputJSON)
		}
	}
}

func TestHandleBodySizeLimit(t *testing.T) {
	policyJSON, err := json.Marshal(testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"policy_dot":` + string(policyJSON) + `,"input":{"age":20}}`
	t.Setenv("MAX_BODY_BYTES", strconv.Itoa(len(payload)))
	response, err := Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() at limit error = %v", err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	t.Setenv("MAX_BODY_BYTES", strconv.Itoa(len(payload)-1))
	response, err = Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() above limit error = %v", err)
	}
	if response.StatusCode != 413 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	var body errorBody
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Code != codeTooLarge {
		t.Fatalf("code = %q, want %q", body.Code, codeTooLarge)
	}
}

func TestHandleLogsDoNotExposePolicyOrInput(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	policyJSON, err := json.Marshal(`digraph P { start [result="secret=\"super-secret-value\""] }`)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"policy_dot":` + string(policyJSON) + `,"input":{"secret_input":"super-secret-value"}}`
	response, err := Handle(context.Background(), request(payload))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, body = %s", response.StatusCode, response.Body)
	}
	logs := output.String()
	if !strings.Contains(logs, "status=200") || !strings.Contains(logs, "duration_ms=") {
		t.Fatalf("unexpected log output: %q", logs)
	}
	if strings.Contains(logs, "super-secret-value") || strings.Contains(logs, "secret_input") {
		t.Fatalf("log leaked policy or input: %q", logs)
	}
}
