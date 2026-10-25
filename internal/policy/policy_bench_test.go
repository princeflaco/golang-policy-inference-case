package policy

import "testing"

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
		parsed, err := Parse(mediumPolicyDOT)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkPolicy = parsed
	}
}

func BenchmarkInferMediumPolicy(b *testing.B) {
	parsed, err := Parse(mediumPolicyDOT)
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
		parsed, err := Parse(mediumPolicyDOT)
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
