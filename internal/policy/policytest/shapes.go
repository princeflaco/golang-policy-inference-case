// Package policytest builds DOT policies and matching inputs of controlled
// shapes, shared by the engine benchmarks and the HTTP load measurement.
// Every generated policy stays inside policy.DefaultLimits().
package policytest

import (
	"fmt"
	"strings"
)

// Shape is a DOT policy paired with the input that traverses it.
type Shape struct {
	DOT   string
	Input map[string]any
}

// Chain returns a linear policy: start -> c1 -> ... -> cN. Each edge carries
// a compound condition that is true for the returned input, and each node
// assigns one variable, so Infer walks the full depth and the output state
// grows by one entry per hop.
func Chain(steps int) Shape {
	var b strings.Builder
	b.WriteString("digraph P {\n\tstart [result=\"origin=1\"]\n")
	prev := "start"
	for i := 1; i <= steps; i++ {
		node := fmt.Sprintf("c%d", i)
		fmt.Fprintf(&b, "\t%s [result=\"v%d=%d\"]\n", node, i, i)
		fmt.Fprintf(&b, "\t%s -> %s [cond=\"x>=0 && (x<1000000 || flag)\"]\n", prev, node)
		prev = node
	}
	b.WriteString("}")
	return Shape{DOT: b.String(), Input: map[string]any{"x": 1.0, "flag": false}}
}

// Fanout returns a policy where start has one outgoing edge per arm. Only
// the last-declared arm is true for the returned input, so Infer evaluates
// every condition — the worst case for declaration-order traversal.
func Fanout(arms int) Shape {
	var b strings.Builder
	b.WriteString("digraph P {\n\tstart\n")
	for i := 0; i < arms; i++ {
		fmt.Fprintf(&b, "\tt%d [result=\"picked=%d\"]\n", i, i)
		fmt.Fprintf(&b, "\tstart -> t%d [cond=\"score==%d && active\"]\n", i, i)
	}
	b.WriteString("}")
	return Shape{
		DOT:   b.String(),
		Input: map[string]any{"score": float64(arms - 1), "active": true},
	}
}

// WideCondition returns a single-edge policy whose condition chains terms
// numeric comparisons with &&. Keep terms below the default expression-depth
// limit: left-associative && nests one level per operand.
func WideCondition(terms int) Shape {
	var expr strings.Builder
	input := make(map[string]any, terms)
	for i := 0; i < terms; i++ {
		if i > 0 {
			expr.WriteString(" && ")
		}
		name := fmt.Sprintf("x%d", i)
		fmt.Fprintf(&expr, "%s>=0", name)
		input[name] = 1.0
	}
	dot := fmt.Sprintf("digraph P {\n\tstart\n\tend [result=\"done=true\"]\n\tstart -> end [cond=\"%s\"]\n}", expr.String())
	return Shape{DOT: dot, Input: input}
}

// Layered returns a DAG of layers tiers with width nodes each. Every node in
// a tier points to every node in the next, so edge count grows quadratically
// and Parse does most of the work. Only the edges into the first node of
// each tier are true for the returned input, giving a deterministic path of
// length layers.
func Layered(layers, width int) Shape {
	var b strings.Builder
	b.WriteString("digraph P {\n\tstart\n")
	prev := []string{"start"}
	for l := 1; l <= layers; l++ {
		current := make([]string, 0, width)
		for w := 0; w < width; w++ {
			node := fmt.Sprintf("l%d_%d", l, w)
			current = append(current, node)
			fmt.Fprintf(&b, "\t%s [result=\"v%d_%d=%d\"]\n", node, l, w, w)
			cond := "skip"
			if w == 0 {
				cond = "ok"
			}
			for _, from := range prev {
				fmt.Fprintf(&b, "\t%s -> %s [cond=\"%s\"]\n", from, node, cond)
			}
		}
		prev = current
	}
	b.WriteString("}")
	return Shape{
		DOT:   b.String(),
		Input: map[string]any{"ok": true, "skip": false},
	}
}
