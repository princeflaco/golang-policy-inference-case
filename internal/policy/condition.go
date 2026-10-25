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

func parseCondition(raw string) (ast.Expr, error) {
	expr, err := goparser.ParseExpr(raw)
	if err != nil {
		return nil, err
	}
	if depth := expressionDepth(expr); depth > maxExpressionDepth() {
		return nil, fmt.Errorf("expression depth %d exceeds limit %d", depth, maxExpressionDepth())
	}
	if err := validateCondition(expr); err != nil {
		return nil, err
	}
	return expr, nil
}

func validateCondition(expr ast.Expr) error {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return validateCondition(value.X)
	case *ast.Ident:
		if value.Name == "true" || value.Name == "false" || identifierPattern.MatchString(value.Name) {
			return nil
		}
		return fmt.Errorf("invalid identifier %q", value.Name)
	case *ast.BinaryExpr:
		switch value.Op {
		case token.EQL, token.NEQ, token.GTR, token.LSS, token.GEQ, token.LEQ:
			if err := validateOperand(value.X); err != nil {
				return err
			}
			return validateOperand(value.Y)
		case token.LAND, token.LOR:
			if err := validateCondition(value.X); err != nil {
				return err
			}
			return validateCondition(value.Y)
		default:
			return fmt.Errorf("operator %s is not supported", value.Op)
		}
	default:
		return errors.New("condition must be a comparison or boolean value")
	}
}

func validateOperand(expr ast.Expr) error {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return validateOperand(value.X)
	case *ast.Ident:
		if identifierPattern.MatchString(value.Name) {
			return nil
		}
		return fmt.Errorf("invalid identifier %q", value.Name)
	case *ast.BasicLit:
		switch value.Kind {
		case token.STRING:
			_, err := unquotePolicyString(value.Value)
			return err
		case token.INT, token.FLOAT:
			if numberPattern.MatchString(value.Value) {
				return nil
			}
			return fmt.Errorf("invalid number literal %q", value.Value)
		default:
			return fmt.Errorf("literal kind %s is not supported", value.Kind)
		}
	case *ast.UnaryExpr:
		if value.Op != token.SUB {
			return fmt.Errorf("operator %s is not supported", value.Op)
		}
		lit, ok := value.X.(*ast.BasicLit)
		if !ok || (lit.Kind != token.INT && lit.Kind != token.FLOAT) {
			return errors.New("negative literals must be numeric")
		}
		if !numberPattern.MatchString(value.Op.String() + lit.Value) {
			return fmt.Errorf("invalid number literal -%s", lit.Value)
		}
		return nil
	default:
		return fmt.Errorf("unsupported operand %T", expr)
	}
}

func evaluateCondition(expr ast.Expr, state map[string]any) (bool, error) {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return evaluateCondition(value.X, state)
	case *ast.Ident:
		raw, err := conditionOperand(value, state)
		if err != nil {
			return false, err
		}
		result, ok := raw.(bool)
		if !ok {
			return false, inferenceError("condition %q is not boolean", value.Name)
		}
		return result, nil
	case *ast.BinaryExpr:
		switch value.Op {
		case token.LAND, token.LOR:
			left, err := evaluateCondition(value.X, state)
			if err != nil {
				return false, err
			}
			if value.Op == token.LAND && !left {
				return false, nil
			}
			if value.Op == token.LOR && left {
				return true, nil
			}
			return evaluateCondition(value.Y, state)
		default:
			left, err := conditionOperand(value.X, state)
			if err != nil {
				return false, err
			}
			right, err := conditionOperand(value.Y, state)
			if err != nil {
				return false, err
			}
			return compareValues(left, right, value.Op)
		}
	default:
		return false, inferenceError("unsupported condition %T", expr)
	}
}

func conditionOperand(expr ast.Expr, state map[string]any) (any, error) {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return conditionOperand(value.X, state)
	case *ast.Ident:
		switch value.Name {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		raw, ok := state[value.Name]
		if !ok {
			return nil, inferenceError("variable %q is not defined", value.Name)
		}
		switch typed := raw.(type) {
		case string, bool, float64:
			return typed, nil
		default:
			return nil, inferenceError("variable %q has unsupported type %T", value.Name, raw)
		}
	case *ast.BasicLit:
		switch value.Kind {
		case token.STRING:
			literal, err := unquotePolicyString(value.Value)
			if err != nil {
				return nil, inferenceError("invalid string literal: %v", err)
			}
			return literal, nil
		case token.INT, token.FLOAT:
			number, err := parseNumber(value.Value)
			if err != nil {
				return nil, inferenceError("invalid number literal: %v", err)
			}
			return number, nil
		default:
			return nil, inferenceError("literal kind %s is not supported", value.Kind)
		}
	case *ast.UnaryExpr:
		if value.Op != token.SUB {
			return nil, inferenceError("operator %s is not supported", value.Op)
		}
		lit, ok := value.X.(*ast.BasicLit)
		if !ok {
			return nil, inferenceError("negative literal must be numeric")
		}
		number, err := parseNumber(value.Op.String() + lit.Value)
		if err != nil {
			return nil, inferenceError("invalid number literal: %v", err)
		}
		return number, nil
	default:
		return nil, inferenceError("unsupported operand %T", expr)
	}
}

func compareValues(left, right any, op token.Token) (bool, error) {
	switch leftValue := left.(type) {
	case string:
		rightValue, ok := right.(string)
		if !ok {
			return false, inferenceError("cannot compare string with %T", right)
		}
		return compareEquality(leftValue, rightValue, op)
	case bool:
		rightValue, ok := right.(bool)
		if !ok {
			return false, inferenceError("cannot compare bool with %T", right)
		}
		return compareEquality(leftValue, rightValue, op)
	case float64:
		rightValue, ok := right.(float64)
		if !ok {
			return false, inferenceError("cannot compare number with %T", right)
		}
		return compareNumbers(leftValue, rightValue, op)
	default:
		return false, inferenceError("unsupported value type %T", left)
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
	case token.LEQ:
		return left <= right, nil
	default:
		return false, inferenceError("unsupported operator %s", op)
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
