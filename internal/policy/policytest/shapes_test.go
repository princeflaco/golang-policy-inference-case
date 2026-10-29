package policytest_test

import (
	"fmt"
	"testing"

	"github.com/princeflaco/golang-policy-inference-case/internal/policy"
	"github.com/princeflaco/golang-policy-inference-case/internal/policy/policytest"
)

func TestShapesParseAndInfer(t *testing.T) {
	tests := []struct {
		name  string
		shape policytest.Shape
		want  map[string]any
	}{
		{"chain", policytest.Chain(30), map[string]any{"v30": 30.0}},
		{"fanout", policytest.Fanout(40), map[string]any{"picked": 39.0}},
		{"wide-condition", policytest.WideCondition(50), map[string]any{"done": true}},
		{"layered", policytest.Layered(6, 6), map[string]any{"v6_0": 0.0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := policy.Parse(tc.shape.DOT, policy.DefaultLimits())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			output, err := parsed.Infer(tc.shape.Input)
			if err != nil {
				t.Fatalf("Infer: %v", err)
			}
			for key, want := range tc.want {
				if got, ok := output[key]; !ok || got != want {
					t.Fatalf("output[%q] = %v (present=%v), want %v", key, got, ok, want)
				}
			}
		})
	}
}

func TestChainAccumulatesAssignmentsAlongThePath(t *testing.T) {
	shape := policytest.Chain(5)
	parsed, err := policy.Parse(shape.DOT, policy.DefaultLimits())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	output, err := parsed.Infer(shape.Input)
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("v%d", i)
		if output[key] != float64(i) {
			t.Fatalf("output[%q] = %v, want %v", key, output[key], float64(i))
		}
	}
}
