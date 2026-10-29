package policy

import (
	"github.com/princeflaco/golang-policy-inference-case/internal/apperr"
	"reflect"
	"strconv"
	"testing"
)

const twoOutcomePolicy = `digraph Policy {
	start [result=""]
	approved [result="approved=true"]
	rejected [result="approved=false"]

	start -> approved [cond="age>=18"]
	start -> rejected [cond="age<18"]
}`

func TestInferApprovesAndRejects(t *testing.T) {
	p, err := parse(twoOutcomePolicy)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{"approved", map[string]any{"age": 20.0}, map[string]any{"age": 20.0, "approved": true}},
		{"rejected", map[string]any{"age": 16.0}, map[string]any{"age": 16.0, "approved": false}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := p.Infer(test.input)
			if err != nil {
				t.Fatalf("Infer() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.expected) {
				t.Fatalf("Infer() = %#v, want %#v", got, test.expected)
			}
		})
	}
}

func TestInferUsesDeclarationOrderWhenBothConditionsAreTrue(t *testing.T) {
	p, err := parse(`digraph P {
		start
		first [result="choice=true"]
		second [result="choice=false"]
		start -> first [cond="age>=18"]
		start -> second [cond="age>0"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := p.Infer(map[string]any{"age": 20.0})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	if got["choice"] != true {
		t.Fatalf("choice = %#v; first declared edge should win", got["choice"])
	}
}

func TestParseRejectsInvalidSubset(t *testing.T) {
	tests := []string{
		`graph P { start -> ok [cond="age>=18"] }`,
		`digraph P { ok [result="approved=true"] }`,
		`digraph P { start -> ok [cond="age+1>18"] ok [result="approved=true"] }`,
		`digraph P { start -> ok [cond="age>=18"] ok -> start [cond="approved==false"] }`,
		`digraph P { start -> ok [cond="age>=18"] start -> ok [cond="age>0"] }`,
	}
	for _, dot := range tests {
		if _, err := parse(dot); err == nil {
			t.Fatalf("Parse(%q) unexpectedly succeeded", dot)
		}
	}
}

func TestInferSupportsStringBooleanAndNumberValues(t *testing.T) {
	p, err := parse(`digraph P {
		start [result="segment=prime,score=720,active=true"]
		prime [result="approved=true"]
		other [result="approved=false"]
		start -> prime [cond="segment==\"prime\""]
		start -> other [cond="active==false"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := p.Infer(map[string]any{"age": 20.0})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	expected := map[string]any{
		"age": 20.0, "segment": "prime", "score": 720.0,
		"active": true, "approved": true,
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Infer() = %#v, want %#v", got, expected)
	}
}

func TestInferUsesInputStringsAndBooleans(t *testing.T) {
	p, err := parse(`digraph P {
		start
		prime [result="approved=true"]
		other [result="approved=false"]
		start -> prime [cond="segment==\"prime\""]
		start -> other [cond="active==false"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := p.Infer(map[string]any{"segment": "prime", "active": false})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	if got["approved"] != true {
		t.Fatalf("approved = %#v", got["approved"])
	}
}

func TestResultParsesQuotedStrings(t *testing.T) {
	p, err := parse(`digraph P {
		start [result="empty=\"\",manual=\"prime,manual\",escaped=\"a\\\"b\""]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := p.Infer(nil)
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	expected := map[string]any{"empty": "", "manual": "prime,manual", "escaped": "a\"b"}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Infer() = %#v, want %#v", got, expected)
	}
}

func TestParseResultParsesEscapes(t *testing.T) {
	assignments, err := parseResult(`line="a\nb",tab="a\tb",slash="a\\b",quote="a\"b"`)
	if err != nil {
		t.Fatalf("parseResult() error = %v", err)
	}
	got := make(map[string]any, len(assignments))
	for _, assignment := range assignments {
		got[assignment.key] = assignment.value
	}
	expected := map[string]any{"line": "a\nb", "tab": "a\tb", "slash": "a\\b", "quote": "a\"b"}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("assignments = %#v, want %#v", got, expected)
	}
}

func TestTypeMismatchesReturnInferenceError(t *testing.T) {
	tests := []struct {
		name  string
		cond  string
		input map[string]any
	}{
		{"string order", `segment>"prime"`, map[string]any{"segment": "x"}},
		{"string to number", `age>"18"`, map[string]any{"age": 20.0}},
		{"bool equality", `active=="true"`, map[string]any{"active": true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dot := `digraph P { start -> ok [cond=` + strconv.Quote(test.cond) + `] ok [result="approved=true"] }`
			p, err := parse(dot)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if _, err := p.Infer(test.input); err == nil {
				t.Fatal("Infer() unexpectedly succeeded")
			}
		})
	}
}

func TestInferAppliesAssignmentsAcrossIntermediateNodes(t *testing.T) {
	p, err := parse(`digraph P {
		start [result="age=17"]
		check [result="age=20"]
		approved [result="approved=true"]
		rejected [result="approved=false"]
		start -> check [cond="age==17"]
		check -> approved [cond="age>=18"]
		check -> rejected [cond="age<18"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := p.Infer(nil)
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	expected := map[string]any{"age": 20.0, "approved": true}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Infer() = %#v, want %#v", got, expected)
	}
}

func TestInferSupportsTypeOverwriteBetweenNodes(t *testing.T) {
	p, err := parse(`digraph P {
		start [result="level=1"]
		upgrade [result="level=\"prime\""]
		done [result="approved=true"]
		start -> upgrade [cond="level==1"]
		upgrade -> done [cond="level==\"prime\""]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := p.Infer(nil)
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	if got["level"] != "prime" || got["approved"] != true {
		t.Fatalf("Infer() = %#v", got)
	}
}

func TestInferStopsWhenNoOutgoingConditionIsTrue(t *testing.T) {
	p, err := parse(`digraph P {
		start [result="approved=true"]
		ok [result="reached=true"]
		start -> ok [cond="age>=18"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := p.Infer(map[string]any{"age": 16.0})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	expected := map[string]any{"age": 16.0, "approved": true}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Infer() = %#v, want %#v", got, expected)
	}
}

func TestInferDoesNotShareStateBetweenCalls(t *testing.T) {
	p, err := parse(`digraph P {
		start [result="visits=1"]
		next [result="visits=2"]
		start -> next [cond="visits==1"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	input := map[string]any{"external": true}
	first, err := p.Infer(input)
	if err != nil {
		t.Fatalf("first Infer() error = %v", err)
	}
	first["visits"] = 99.0
	second, err := p.Infer(input)
	if err != nil {
		t.Fatalf("second Infer() error = %v", err)
	}
	if input["visits"] != nil || second["visits"] != 2.0 {
		t.Fatalf("input = %#v, second = %#v", input, second)
	}
}

func TestInferSupportsCompoundConditionsAndPrecedence(t *testing.T) {
	p, err := parse(`digraph P {
		start
		approved [result="approved=true"]
		review [result="approved=false,status=\"review\""]
		rejected [result="approved=false"]
		start -> approved [cond="age>=18 && score>700"]
		start -> review [cond="age>=18 && score<=700"]
		start -> rejected [cond="age<18"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := p.Infer(map[string]any{"age": 20.0, "score": 690.0})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	if got["status"] != "review" || got["approved"] != false {
		t.Fatalf("Infer() = %#v", got)
	}
}

func TestAndShortCircuitsMissingVariable(t *testing.T) {
	p, err := parse(`digraph P {
		start -> ok [cond="age>18 && score>700"]
		ok [result="approved=true"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := p.Infer(map[string]any{"age": 16.0})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	if got["approved"] != nil {
		t.Fatalf("Infer() = %#v", got)
	}
}

func TestOrShortCircuitsMissingVariable(t *testing.T) {
	p, err := parse(`digraph P {
		start -> ok [cond="age>=18 || score>700"]
		ok [result="approved=true"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := p.Infer(map[string]any{"age": 20.0})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	if got["approved"] != true {
		t.Fatalf("Infer() = %#v", got)
	}
}

func TestCompoundConditionAccessedMissingVariableErrors(t *testing.T) {
	p, err := parse(`digraph P {
		start -> ok [cond="age>=18 && score>700"]
		ok [result="approved=true"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if _, err := p.Infer(map[string]any{"age": 20.0}); err == nil {
		t.Fatal("Infer() unexpectedly succeeded")
	}
}

func TestCompoundConditionPreservesLeftError(t *testing.T) {
	p, err := parse(`digraph P {
		start -> ok [cond="score>700 || age>=18"]
		ok [result="approved=true"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if _, err := p.Infer(map[string]any{"age": 20.0}); err == nil {
		t.Fatal("Infer() unexpectedly succeeded")
	}
}

func TestCompoundConditionParenthesesChangeGrouping(t *testing.T) {
	p, err := parse(`digraph P {
		start
		ok [result="approved=true"]
		no [result="approved=false"]
		start -> ok [cond="(age>=18 || score>700) && active==false"]
		start -> no [cond="active==true"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := p.Infer(map[string]any{"age": 20.0, "active": true})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	if got["approved"] != false {
		t.Fatalf("Infer() = %#v", got)
	}
}

func TestParseValidatesUnreachablePolicySections(t *testing.T) {
	_, err := parse(`digraph P {
		start -> ok [cond="age>=18"]
		ok [result="approved=true"]
		unreachable -> bad [cond="age+1>18"]
	}`)
	if err == nil {
		t.Fatal("parse() unexpectedly accepted an invalid condition on an unreachable edge")
	}
}

func TestParseRejectsProhibitedConditionForms(t *testing.T) {
	tests := []string{
		`age+1>18`,
		`f(age)`,
		`user.age==18`,
		`items[0]==18`,
		`age==[18]`,
		`!active`,
		`age>=18 ||`,
	}
	for _, condition := range tests {
		t.Run(condition, func(t *testing.T) {
			dot := `digraph P { start -> ok [cond=` + strconv.Quote(condition) + `] ok [result="approved=true"] }`
			if _, err := parse(dot); err == nil {
				t.Fatalf("Parse(%q) unexpectedly succeeded", condition)
			}
		})
	}
}

func TestParseRejectsProhibitedDOTForms(t *testing.T) {
	tests := []string{
		`digraph P { ok [result="approved=true"] }`,
		`digraph P { start -> ok [cond="age>=18"] start -> bad }`,
		`digraph P { start -> ok [cond=""] ok [result="approved=true"] }`,
		`digraph P { node [shape=box] start -> ok [cond="age>=18"] }`,
		`digraph P { subgraph S { start -> ok } ok [result="approved=true"] }`,
		`digraph P { start:port -> ok [cond="age>=18"] ok [result="approved=true"] }`,
		`digraph P { start -> ok -> done [cond="age>=18"] }`,
		`digraph P { start [result="approved=true",result="approved=false"] }`,
		`digraph P { start -> ok [cond="age>=18",cond="age<18"] }`,
	}
	for _, dot := range tests {
		if _, err := parse(dot); err == nil {
			t.Fatalf("Parse(%q) unexpectedly succeeded", dot)
		}
	}
}

func TestParseRejectsInvalidAssignments(t *testing.T) {
	tests := []string{
		`approved=`,
		`approved`,
		`approved=true,approved=false`,
		`segment="unterminated`,
		`segment="bad\rescape"`,
		`segment=hello world`,
		`score=1e3`,
		`score=.5`,
		`score=1.`,
	}
	for _, result := range tests {
		t.Run(result, func(t *testing.T) {
			dot := `digraph P { start [result=` + strconv.Quote(result) + `] }`
			if _, err := parse(dot); err == nil {
				t.Fatalf("Parse(%q) unexpectedly succeeded", result)
			}
		})
	}
}

func TestPolicyLimits(t *testing.T) {
	tests := []struct {
		name   string
		dot    string
		atMost func(*Limits, int)
		limit  int
	}{
		{"nodes", `digraph P { start -> a [cond="true"] a -> b [cond="true"] }`, func(l *Limits, n int) { l.MaxPolicyNodes = n }, 3},
		{"edges", `digraph P { start -> a [cond="true"] start -> b [cond="false"] }`, func(l *Limits, n int) { l.MaxPolicyEdges = n }, 2},
		{"condition bytes", `digraph P { start -> ok [cond="age>=18"] }`, func(l *Limits, n int) { l.MaxConditionBytes = n }, 7},
		{"expression depth", `digraph P { start -> ok [cond="age>=18 && score>700 && active"] }`, func(l *Limits, n int) { l.MaxExpressionDepth = n }, 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limits := DefaultLimits()
			test.atMost(&limits, test.limit)
			if _, err := Parse(test.dot, limits); err != nil {
				t.Fatalf("Parse() at limit error = %v", err)
			}
			test.atMost(&limits, test.limit-1)
			_, err := Parse(test.dot, limits)
			if apiErr := apperr.From(err); apiErr == nil || apiErr.Code != apperr.InvalidPolicy {
				t.Fatalf("Parse() above limit error = %v, want invalid_policy", err)
			}
		})
	}
}

func TestParseRejectsNonPositiveLimits(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxPolicyEdges = 0
	_, err := Parse(`digraph P { start }`, limits)
	if err == nil || apperr.From(err) != nil {
		t.Fatalf("Parse() error = %v, want an uncoded programming error", err)
	}
}

func TestParseRejectsTopLevelAttrAndUnsupportedDotEscape(t *testing.T) {
	for name, dot := range map[string]string{
		"top-level attr": `digraph { rankdir=LR start -> ok [cond="true"] }`,
		"bad escape":     "digraph { start -> ok [cond=\"true\"] ok [result=\"a\rb\"] }",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(dot); err == nil {
				t.Fatal("parse() expected error")
			}
		})
	}
}

func TestParseResultAcceptsDigitLeadingSimpleStrings(t *testing.T) {
	got, err := parseResult("a=123abc, b=1_000, c=1-2, d=x1")
	if err != nil {
		t.Fatalf("parseResult() error = %v", err)
	}
	want := []any{"123abc", "1_000", "1-2", "x1"}
	for i, assignment := range got {
		if assignment.value != want[i] {
			t.Fatalf("assignment[%d] = %#v, want %#v", i, assignment.value, want[i])
		}
	}
	for _, literal := range []string{"1e3", ".5", "1."} {
		if _, err := parseResult("x=" + literal); err == nil {
			t.Fatalf("parseResult(x=%s) expected error", literal)
		}
	}
}

func parse(dot string) (*Policy, error) {
	return Parse(dot, DefaultLimits())
}

func TestConditionSemantics(t *testing.T) {
	tests := []struct {
		name    string
		cond    string
		input   map[string]any
		matched bool
		code    apperr.Code
	}{
		{"true literal", `true`, nil, true, ""},
		{"false literal", `false`, nil, false, ""},
		{"boolean variable", `active`, map[string]any{"active": true}, true, ""},
		{"negative literal", `score>-0.5`, map[string]any{"score": 0.0}, true, ""},
		{"parenthesised operand", `(age)>=18`, map[string]any{"age": 18.0}, true, ""},
		{"literal on the left", `18<=age`, map[string]any{"age": 20.0}, true, ""},
		{"string inequality", `segment!="prime"`, map[string]any{"segment": "basic"}, true, ""},
		{"non-boolean identifier", `score`, map[string]any{"score": 1.0}, false, apperr.InferenceError},
		{"missing variable", `age>=18`, nil, false, apperr.InferenceError},
		{"string ordering", `segment>"a"`, map[string]any{"segment": "b"}, false, apperr.InferenceError},
		{"bool vs number", `active==1`, map[string]any{"active": true}, false, apperr.InferenceError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dot := `digraph P { start -> ok [cond=` + strconv.Quote(test.cond) + `] ok [result="matched=true"] }`
			p, err := parse(dot)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			got, err := p.Infer(test.input)
			if test.code != "" {
				if apiErr := apperr.From(err); apiErr == nil || apiErr.Code != test.code {
					t.Fatalf("Infer() error = %v, want code %s", err, test.code)
				}
				return
			}
			if err != nil {
				t.Fatalf("Infer() error = %v", err)
			}
			if matched := got["matched"] == true; matched != test.matched {
				t.Fatalf("matched = %v, want %v", matched, test.matched)
			}
		})
	}
}

func TestParseRejectsConditionsWithInvalidPolicyCode(t *testing.T) {
	for _, cond := range []string{`age+1>18`, `-age>1`, `--1<0`, `"a"`, `1.5e3>0`, `'a'=="a"`, `segment=="bad\r"`} {
		t.Run(cond, func(t *testing.T) {
			_, err := parse(`digraph P { start -> ok [cond=` + strconv.Quote(cond) + `] }`)
			if apiErr := apperr.From(err); apiErr == nil || apiErr.Code != apperr.InvalidPolicy {
				t.Fatalf("Parse() error = %v, want invalid_policy", err)
			}
		})
	}
}
