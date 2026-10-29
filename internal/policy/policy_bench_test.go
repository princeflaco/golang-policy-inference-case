package policy

import (
	"testing"

	"github.com/princeflaco/golang-policy-inference-case/internal/policy/policytest"
)

const mediumPolicyDOT = `digraph P {
	start [result="tier=base"]
	score [result="score=710"]
	segment [result="segment=\"basic\""]
	high [result="approved=true"]
	low [result="approved=false,reason=\"score\""]

	start -> score [cond="age>=18"]
	start -> segment [cond="age<18"]
	score -> high [cond="score>700 && segment==\"prime\" && active"]
	score -> low [cond="score<=700 || segment==\"basic\""]
	segment -> high [cond="segment==\"prime\" && active"]
	segment -> low [cond="segment==\"basic\""]
}`

var (
	benchmarkPolicy *Policy
	benchmarkOutput map[string]any
)

var mediumPolicyInput = map[string]any{
	"age":     20.0,
	"score":   700.0,
	"segment": "prime",
	"active":  true,
}

func BenchmarkParseMediumPolicy(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parsed, err := parse(mediumPolicyDOT)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkPolicy = parsed
	}
}

func BenchmarkInferMediumPolicy(b *testing.B) {
	parsed, err := parse(mediumPolicyDOT)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output, err := parsed.Infer(mediumPolicyInput)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkOutput = output
	}
}

func BenchmarkParseAndInferMediumPolicy(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parsed, err := parse(mediumPolicyDOT)
		if err != nil {
			b.Fatal(err)
		}
		output, err := parsed.Infer(mediumPolicyInput)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkPolicy = parsed
		benchmarkOutput = output
	}
}

// benchmarkShapes covers the policy shapes a real workload can throw at the
// engine: deep paths, wide condition fan-out, long boolean expressions and
// wide DAGs where Parse dominates. medium is repeated here so the shape
// table alone tells the whole story.
var benchmarkShapes = []struct {
	name  string
	shape policytest.Shape
}{
	{"medium", policytest.Shape{DOT: mediumPolicyDOT, Input: mediumPolicyInput}},
	{"chain-100", policytest.Chain(100)},
	{"fanout-50", policytest.Fanout(50)},
	{"wide-condition-50", policytest.WideCondition(50)},
	{"layered-6x6", policytest.Layered(6, 6)},
	{"layered-9x9", policytest.Layered(9, 9)},
}

func BenchmarkParseShapes(b *testing.B) {
	for _, shape := range benchmarkShapes {
		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				parsed, err := parse(shape.shape.DOT)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkPolicy = parsed
			}
		})
	}
}

func BenchmarkInferShapes(b *testing.B) {
	for _, shape := range benchmarkShapes {
		parsed, err := parse(shape.shape.DOT)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				output, err := parsed.Infer(shape.shape.Input)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkOutput = output
			}
		})
	}
}

func BenchmarkParseAndInferShapes(b *testing.B) {
	for _, shape := range benchmarkShapes {
		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				parsed, err := parse(shape.shape.DOT)
				if err != nil {
					b.Fatal(err)
				}
				output, err := parsed.Infer(shape.shape.Input)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkPolicy = parsed
				benchmarkOutput = output
			}
		})
	}
}
