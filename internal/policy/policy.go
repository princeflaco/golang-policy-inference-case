package policy

import (
	"fmt"
	"go/ast"
	"regexp"
	"sort"
	"strings"

	godot "gonum.org/v1/gonum/graph/formats/dot"
	gvast "gonum.org/v1/gonum/graph/formats/dot/ast"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Kind identifies whether a failure belongs to the policy or to inference.
type Kind string

const (
	InvalidPolicy   Kind = "invalid_policy"
	InferenceError  Kind = "inference_error"
	identifierStart      = "start"
)

// Error carries the class of error needed by the HTTP layer.
type Error struct {
	Kind    Kind
	Message string
}

func (e *Error) Error() string { return e.Message }

func invalidPolicy(format string, args ...any) error {
	return &Error{Kind: InvalidPolicy, Message: fmt.Sprintf(format, args...)}
}

func inferenceError(format string, args ...any) error {
	return &Error{Kind: InferenceError, Message: fmt.Sprintf(format, args...)}
}

// Policy is the executable model built from a DOT graph.
type Policy struct {
	nodes map[string]*node
	start string
}

type node struct {
	assignments []assignment
	edges       []*edge
}

type assignment struct {
	key   string
	value any
}

type edge struct {
	to   string
	expr ast.Expr
}

// Parse converts the approved subset of DOT into an executable policy.
func Parse(dot string) (*Policy, error) {
	file, err := godot.ParseString(dot)
	if err != nil {
		return nil, invalidPolicy("invalid DOT: %v", err)
	}
	if len(file.Graphs) != 1 {
		return nil, invalidPolicy("policy must contain exactly one graph")
	}

	graph := file.Graphs[0]
	if !graph.Directed {
		return nil, invalidPolicy("policy graph must be directed")
	}

	p := &Policy{nodes: make(map[string]*node)}
	declared := make(map[string]bool)
	seenEdges := make(map[string]bool)
	edgeCount := 0
	for _, stmt := range graph.Stmts {
		if _, ok := stmt.(*gvast.EdgeStmt); ok {
			edgeCount++
		}
	}
	if edgeCount > maxPolicyEdges() {
		return nil, invalidPolicy("policy has %d edges; limit is %d", edgeCount, maxPolicyEdges())
	}

	getNode := func(name string) *node {
		if existing, ok := p.nodes[name]; ok {
			return existing
		}
		n := &node{}
		p.nodes[name] = n
		return n
	}

	for i, stmt := range graph.Stmts {
		switch s := stmt.(type) {
		case *gvast.NodeStmt:
			name, err := vertexName(s.Node)
			if err != nil {
				return nil, err
			}
			if declared[name] {
				return nil, invalidPolicy("node %q declared more than once", name)
			}
			declared[name] = true

			if err := rejectDuplicateAttrs(s.Attrs); err != nil {
				return nil, err
			}
			n := getNode(name)
			if len(p.nodes) > maxPolicyNodes() {
				return nil, invalidPolicy("policy has more than %d nodes", maxPolicyNodes())
			}
			for _, attr := range s.Attrs {
				if attr.Key != "result" {
					continue
				}
				rawResult, err := dotAttrValue(attr.Val)
				if err != nil {
					return nil, invalidPolicy("node %q has invalid result: %v", name, err)
				}
				assignments, err := parseResult(rawResult)
				if err != nil {
					return nil, err
				}
				n.assignments = assignments
			}

		case *gvast.EdgeStmt:
			from, err := vertexName(s.From)
			if err != nil {
				return nil, err
			}
			if s.To == nil {
				return nil, invalidPolicy("edge statement %d has no destination", i+1)
			}
			if s.To.To != nil {
				return nil, invalidPolicy("chained edge statements are not supported")
			}
			if !s.To.Directed {
				return nil, invalidPolicy("edges must be directed")
			}
			to, err := vertexName(s.To.Vertex)
			if err != nil {
				return nil, err
			}
			edgeKey := from + "\x00" + to
			if seenEdges[edgeKey] {
				return nil, invalidPolicy("edge %q -> %q declared more than once", from, to)
			}
			seenEdges[edgeKey] = true

			if err := rejectDuplicateAttrs(s.Attrs); err != nil {
				return nil, err
			}
			var rawCond string
			for _, attr := range s.Attrs {
				if attr.Key == "cond" {
					rawCond = attr.Val
				}
			}
			if strings.TrimSpace(rawCond) == "" {
				return nil, invalidPolicy("edge %q -> %q must declare a non-empty cond", from, to)
			}
			rawCond, err = dotAttrValue(rawCond)
			if err != nil {
				return nil, invalidPolicy("edge %q -> %q has invalid cond: %v", from, to, err)
			}
			if len(rawCond) > maxConditionBytes() {
				return nil, invalidPolicy("edge %q -> %q cond exceeds %d bytes", from, to, maxConditionBytes())
			}
			expr, err := parseCondition(rawCond)
			if err != nil {
				return nil, invalidPolicy("edge %q -> %q has invalid cond: %v", from, to, err)
			}

			getNode(to)
			fromNode := getNode(from)
			if len(p.nodes) > maxPolicyNodes() {
				return nil, invalidPolicy("policy has more than %d nodes", maxPolicyNodes())
			}
			fromNode.edges = append(fromNode.edges, &edge{to: to, expr: expr})

		case *gvast.Subgraph:
			return nil, invalidPolicy("subgraphs are not supported")
		case *gvast.AttrStmt:
			return nil, invalidPolicy("default graph, node, or edge attributes are not supported")
		case *gvast.Attr:
			return nil, invalidPolicy("top-level attributes are not supported")
		default:
			return nil, invalidPolicy("unsupported DOT statement %T", stmt)
		}
	}

	if _, ok := p.nodes[identifierStart]; !ok {
		return nil, invalidPolicy("policy graph must contain a start node")
	}
	p.start = identifierStart

	if hasCycle(p.nodes) {
		return nil, invalidPolicy("policy graph must not contain cycles")
	}
	return p, nil
}

// Infer executes one deterministic path and returns a new state. The input map
// is not modified.
func (p *Policy) Infer(input map[string]any) (map[string]any, error) {
	state := make(map[string]any, len(input)+4)
	for key, value := range input {
		if !identifierPattern.MatchString(key) {
			return nil, inferenceError("input key %q is not a valid identifier", key)
		}
		state[key] = value
	}

	current := p.start
	for {
		n := p.nodes[current]
		for _, assignment := range n.assignments {
			state[assignment.key] = assignment.value
		}

		var next string
		for _, edge := range n.edges {
			matched, err := evaluateCondition(edge.expr, state)
			if err != nil {
				return nil, err
			}
			if matched {
				next = edge.to
				break
			}
		}
		if next == "" {
			return state, nil
		}
		current = next
	}
}

func vertexName(v gvast.Vertex) (string, error) {
	n, ok := v.(*gvast.Node)
	if !ok {
		return "", invalidPolicy("subgraph vertices are not supported")
	}
	if n.Port != nil {
		return "", invalidPolicy("node ports are not supported")
	}
	if !identifierPattern.MatchString(n.ID) {
		return "", invalidPolicy("invalid identifier %q", n.ID)
	}
	return n.ID, nil
}

func rejectDuplicateAttrs(attrs []*gvast.Attr) error {
	seen := make(map[string]bool, len(attrs))
	for _, attr := range attrs {
		if seen[attr.Key] {
			return invalidPolicy("attribute %q declared more than once", attr.Key)
		}
		seen[attr.Key] = true
	}
	return nil
}

func dotAttrValue(raw string) (string, error) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return raw, nil
	}
	inner := raw[1 : len(raw)-1]
	var value strings.Builder
	for i := 0; i < len(inner); i++ {
		if inner[i] != '\\' {
			value.WriteByte(inner[i])
			continue
		}
		i++
		if i >= len(inner) {
			return "", fmt.Errorf("unterminated escape in DOT value %q", raw)
		}
		var decoded byte
		switch inner[i] {
		case '"':
			decoded = '"'
		case '\\':
			decoded = '\\'
		case 'n':
			decoded = '\n'
		case 't':
			decoded = '\t'
		default:
			return "", fmt.Errorf("unsupported escape \\%c in DOT value", inner[i])
		}
		value.WriteByte(decoded)
	}
	return value.String(), nil
}

func hasCycle(nodes map[string]*node) bool {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := make(map[string]int, len(nodes))
	var visit func(string) bool
	visit = func(name string) bool {
		switch state[name] {
		case visiting:
			return true
		case visited:
			return false
		}
		state[name] = visiting
		for _, edge := range nodes[name].edges {
			if visit(edge.to) {
				return true
			}
		}
		state[name] = visited
		return false
	}
	for _, name := range orderedNodeNames(nodes) {
		if visit(name) {
			return true
		}
	}
	return false
}

func orderedNodeNames(nodes map[string]*node) []string {
	names := make([]string, 0, len(nodes))
	for name := range nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
