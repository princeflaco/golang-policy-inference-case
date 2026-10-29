package policy

import (
	"errors"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"math"
	"regexp"
	"strconv"
)

var numberPattern = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)

// condition is a compiled edge condition. Compilation enforces the whole
// condition grammar once, at parse time, so evaluation only ever meets the
// node kinds below and can fail only on state: missing variables or
// incompatible types.
type condition interface {
	eval(state map[string]any) (bool, error)
}

// operand is one side of a comparison, or a value used directly as a
// boolean condition.
type operand interface {
	value(state map[string]any) (any, error)
}

type (
	andCondition     struct{ left, right condition }
	orCondition      struct{ left, right condition }
	compareCondition struct {
		op          token.Token
		left, right operand
	}
	// boolCondition is an identifier or literal used as a condition on its
	// own, e.g. `active` or `true`.
	boolCondition struct {
		name    string
		operand operand
	}
	literal  struct{ v any }
	variable struct{ name string }
)

// parseCondition reuses go/parser for precedence and grouping, bounds the
// expression depth, and compiles the accepted subset.
func parseCondition(raw string, maxDepth int) (condition, error) {
	expr, err := goparser.ParseExpr(raw)
	if err != nil {
		return nil, err
	}
	if depth := expressionDepth(expr); depth > maxDepth {
		return nil, fmt.Errorf("expression depth %d exceeds limit %d", depth, maxDepth)
	}
	return compileCondition(expr)
}

func compileCondition(expr ast.Expr) (condition, error) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return compileCondition(e.X)
	case *ast.Ident:
		op, err := compileIdent(e)
		if err != nil {
			return nil, err
		}
		return boolCondition{name: e.Name, operand: op}, nil
	case *ast.BinaryExpr:
		switch e.Op {
		case token.LAND, token.LOR:
			left, err := compileCondition(e.X)
			if err != nil {
				return nil, err
			}
			right, err := compileCondition(e.Y)
			if err != nil {
				return nil, err
			}
			if e.Op == token.LAND {
				return andCondition{left, right}, nil
			}
			return orCondition{left, right}, nil
		case token.EQL, token.NEQ, token.GTR, token.LSS, token.GEQ, token.LEQ:
			left, err := compileOperand(e.X)
			if err != nil {
				return nil, err
			}
			right, err := compileOperand(e.Y)
			if err != nil {
				return nil, err
			}
			return compareCondition{op: e.Op, left: left, right: right}, nil
		default:
			return nil, fmt.Errorf("operator %s is not supported", e.Op)
		}
	default:
		return nil, errors.New("condition must be a comparison or boolean value")
	}
}

func compileOperand(expr ast.Expr) (operand, error) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return compileOperand(e.X)
	case *ast.Ident:
		return compileIdent(e)
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			s, err := unquotePolicyString(e.Value)
			if err != nil {
				return nil, err
			}
			return literal{s}, nil
		case token.INT, token.FLOAT:
			return numberLiteral(e.Value)
		default:
			return nil, fmt.Errorf("literal kind %s is not supported", e.Kind)
		}
	case *ast.UnaryExpr:
		if e.Op != token.SUB {
			return nil, fmt.Errorf("operator %s is not supported", e.Op)
		}
		lit, ok := e.X.(*ast.BasicLit)
		if !ok || (lit.Kind != token.INT && lit.Kind != token.FLOAT) {
			return nil, errors.New("negative literals must be numeric")
		}
		return numberLiteral("-" + lit.Value)
	default:
		return nil, fmt.Errorf("unsupported operand %T", expr)
	}
}

func compileIdent(e *ast.Ident) (operand, error) {
	switch e.Name {
	case "true":
		return literal{true}, nil
	case "false":
		return literal{false}, nil
	}
	if !identifierPattern.MatchString(e.Name) {
		return nil, fmt.Errorf("invalid identifier %q", e.Name)
	}
	return variable{e.Name}, nil
}

func numberLiteral(raw string) (operand, error) {
	if !numberPattern.MatchString(raw) {
		return nil, fmt.Errorf("invalid number literal %q", raw)
	}
	number, err := parseNumber(raw)
	if err != nil {
		return nil, err
	}
	return literal{number}, nil
}

func (c andCondition) eval(state map[string]any) (bool, error) {
	left, err := c.left.eval(state)
	if err != nil || !left {
		return false, err
	}
	return c.right.eval(state)
}

func (c orCondition) eval(state map[string]any) (bool, error) {
	left, err := c.left.eval(state)
	if err != nil || left {
		return left, err
	}
	return c.right.eval(state)
}

func (c compareCondition) eval(state map[string]any) (bool, error) {
	left, err := c.left.value(state)
	if err != nil {
		return false, err
	}
	right, err := c.right.value(state)
	if err != nil {
		return false, err
	}
	return compareValues(left, right, c.op)
}

func (c boolCondition) eval(state map[string]any) (bool, error) {
	raw, err := c.operand.value(state)
	if err != nil {
		return false, err
	}
	result, ok := raw.(bool)
	if !ok {
		return false, inferenceError("condition %q is not boolean", c.name)
	}
	return result, nil
}

func (l literal) value(map[string]any) (any, error) { return l.v, nil }

func (v variable) value(state map[string]any) (any, error) {
	raw, ok := state[v.name]
	if !ok {
		return nil, inferenceError("variable %q is not defined", v.name)
	}
	return raw, nil
}

func compareValues(left, right any, op token.Token) (bool, error) {
	switch leftValue := left.(type) {
	case string:
		rightValue, ok := right.(string)
		if !ok {
			return false, inferenceError("cannot compare string with %s", typeName(right))
		}
		return compareEquality(leftValue, rightValue, op)
	case bool:
		rightValue, ok := right.(bool)
		if !ok {
			return false, inferenceError("cannot compare bool with %s", typeName(right))
		}
		return compareEquality(leftValue, rightValue, op)
	case float64:
		rightValue, ok := right.(float64)
		if !ok {
			return false, inferenceError("cannot compare number with %s", typeName(right))
		}
		return compareNumbers(leftValue, rightValue, op)
	default:
		return false, inferenceError("unsupported value type %s", typeName(left))
	}
}

func compareEquality[T comparable](left, right T, op token.Token) (bool, error) {
	switch op {
	case token.EQL:
		return left == right, nil
	case token.NEQ:
		return left != right, nil
	default:
		return false, inferenceError("operator %s requires numbers", op)
	}
}

func compareNumbers(left, right float64, op token.Token) (bool, error) {
	switch op {
	case token.EQL:
		return left == right, nil
	case token.NEQ:
		return left != right, nil
	case token.GTR:
		return left > right, nil
	case token.LSS:
		return left < right, nil
	case token.GEQ:
		return left >= right, nil
	default: // token.LEQ: compileCondition admits only the six comparison operators
		return left <= right, nil
	}
}

// typeName names a state value in policy vocabulary rather than Go's.
func typeName(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case bool:
		return "bool"
	case float64:
		return "number"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func parseNumber(raw string) (float64, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, fmt.Errorf("invalid finite number %q", raw)
	}
	return value, nil
}

func expressionDepth(expr ast.Expr) int {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return 1 + expressionDepth(value.X)
	case *ast.UnaryExpr:
		return 1 + expressionDepth(value.X)
	case *ast.BinaryExpr:
		return 1 + max(expressionDepth(value.X), expressionDepth(value.Y))
	case *ast.CallExpr:
		depth := expressionDepth(value.Fun)
		for _, arg := range value.Args {
			depth = max(depth, expressionDepth(arg))
		}
		return 1 + depth
	case *ast.SelectorExpr:
		return 1 + expressionDepth(value.X)
	case *ast.IndexExpr:
		return 1 + max(expressionDepth(value.X), expressionDepth(value.Index))
	case *ast.CompositeLit:
		depth := 0
		for _, element := range value.Elts {
			depth = max(depth, expressionDepth(element))
		}
		return 1 + depth
	default:
		return 1
	}
}
