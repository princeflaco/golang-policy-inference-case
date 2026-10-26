package policy

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var simpleStringPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func parseResult(raw string) ([]assignment, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	parts, err := splitAssignments(trimmed)
	if err != nil {
		return nil, err
	}
	assignments := make([]assignment, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		key, literal, err := splitAssignment(part)
		if err != nil {
			return nil, err
		}
		if !identifierPattern.MatchString(key) {
			return nil, invalidPolicy("invalid assignment key %q", key)
		}
		if seen[key] {
			return nil, invalidPolicy("assignment key %q declared more than once", key)
		}
		seen[key] = true

		value, err := parseAssignmentLiteral(literal)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, assignment{key: key, value: value})
	}
	return assignments, nil
}

func splitAssignments(raw string) ([]string, error) {
	var parts []string
	start := 0
	inString := false
	escaped := false
	for i, r := range raw {
		if escaped {
			escaped = false
			continue
		}
		if inString && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if r == ',' && !inString {
			parts = append(parts, raw[start:i])
			start = i + 1
		}
	}
	if inString || escaped {
		return nil, invalidPolicy("unterminated quoted string in result")
	}
	parts = append(parts, raw[start:])
	return parts, nil
}

func splitAssignment(raw string) (string, string, error) {
	inString := false
	escaped := false
	for i, r := range raw {
		if escaped {
			escaped = false
			continue
		}
		if inString && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if r == '=' && !inString {
			key := strings.TrimSpace(raw[:i])
			literal := strings.TrimSpace(raw[i+1:])
			if key == "" || literal == "" {
				return "", "", invalidPolicy("invalid assignment %q", strings.TrimSpace(raw))
			}
			return key, literal, nil
		}
	}
	return "", "", invalidPolicy("invalid assignment %q", strings.TrimSpace(raw))
}

func parseAssignmentLiteral(literal string) (any, error) {
	switch literal {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if strings.HasPrefix(literal, `"`) {
		value, err := unquotePolicyString(literal)
		if err != nil {
			return nil, invalidPolicy("invalid string literal %q", literal)
		}
		return value, nil
	}
	if numberPattern.MatchString(literal) {
		value, err := parseNumber(literal)
		if err != nil {
			return nil, invalidPolicy("invalid number literal %q", literal)
		}
		return value, nil
	}
	if looksLikeNumber(literal) {
		return nil, invalidPolicy("invalid number literal %q", literal)
	}
	if simpleStringPattern.MatchString(literal) {
		return literal, nil
	}
	return nil, invalidPolicy("invalid assignment literal %q", literal)
}

func unquotePolicyString(literal string) (string, error) {
	if len(literal) < 2 || literal[0] != '"' || literal[len(literal)-1] != '"' {
		return "", fmt.Errorf("string literal must be quoted")
	}
	for i := 1; i < len(literal)-1; i++ {
		if literal[i] != '\\' {
			continue
		}
		i++
		if i >= len(literal)-1 {
			return "", fmt.Errorf("unterminated escape")
		}
		switch literal[i] {
		case '"', '\\', 'n', 't':
		default:
			return "", fmt.Errorf("unsupported escape \\%c", literal[i])
		}
	}
	value, err := strconv.Unquote(literal)
	if err != nil {
		return "", err
	}
	return value, nil
}

func looksLikeNumber(literal string) bool {
	if literal == "" {
		return false
	}
	first := literal[0]
	if !(first >= '0' && first <= '9' || first == '-' && len(literal) > 1 && literal[1] >= '0' && literal[1] <= '9') {
		return false
	}
	for i := 0; i < len(literal); i++ {
		c := literal[i]
		if c >= '0' && c <= '9' || c == '.' || c == 'e' || c == 'E' {
			continue
		}
		if (c == '-' || c == '+') && i == 0 {
			continue
		}
		return false
	}
	return true
}
