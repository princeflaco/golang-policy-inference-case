package policy

import (
	"regexp"
	"strings"
)

var simpleStringPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func parseResult(raw string) ([]assignment, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	parts, err := splitOutsideQuotes(trimmed, ',', -1)
	if err != nil {
		return nil, invalidPolicy("invalid result: %v", err)
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

func splitAssignment(raw string) (string, string, error) {
	sides, err := splitOutsideQuotes(raw, '=', 2)
	if err != nil || len(sides) != 2 {
		return "", "", invalidPolicy("invalid assignment %q", strings.TrimSpace(raw))
	}
	key, literal := strings.TrimSpace(sides[0]), strings.TrimSpace(sides[1])
	if key == "" || literal == "" {
		return "", "", invalidPolicy("invalid assignment %q", strings.TrimSpace(raw))
	}
	return key, literal, nil
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
