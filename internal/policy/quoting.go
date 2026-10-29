package policy

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// This file owns every quote- and escape-aware read in the policy language:
// DOT attribute values, string literals in conditions and assignments, and
// splitting result text on separators that sit outside quoted strings.
// All of them share one escape subset: \" \\ \n \t.

// decodeEscapes decodes the shared escape subset and rejects any other
// backslash sequence.
func decodeEscapes(s string) (string, error) {
	if !strings.Contains(s, `\`) {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			return "", errors.New("unterminated escape")
		}
		switch s[i] {
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		default:
			return "", fmt.Errorf("unsupported escape \\%c", s[i])
		}
	}
	return b.String(), nil
}

// dotAttrValue unwraps a DOT attribute value. Unquoted IDs are returned as
// is; quoted strings have their quotes removed and escapes decoded.
func dotAttrValue(raw string) (string, error) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return raw, nil
	}
	return decodeEscapes(raw[1 : len(raw)-1])
}

// unquotePolicyString reads a double-quoted string literal used in a
// condition or an assignment.
func unquotePolicyString(literal string) (string, error) {
	if len(literal) < 2 || literal[0] != '"' || literal[len(literal)-1] != '"' {
		return "", errors.New("string literal must be quoted")
	}
	inner := literal[1 : len(literal)-1]
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '\\':
			i++
		case '"', '\n':
			return "", errors.New("string literal contains an unescaped quote or newline")
		}
	}
	if !utf8.ValidString(inner) {
		return "", errors.New("string literal is not valid UTF-8")
	}
	return decodeEscapes(inner)
}

// splitOutsideQuotes splits s on sep wherever sep is not inside a
// double-quoted string, returning at most n parts (n < 0 means no limit).
func splitOutsideQuotes(s string, sep byte, n int) ([]string, error) {
	var parts []string
	start := 0
	inString := false
	for i := 0; i < len(s); i++ {
		switch {
		case inString && s[i] == '\\':
			i++
		case s[i] == '"':
			inString = !inString
		case s[i] == sep && !inString && (n < 0 || len(parts) < n-1):
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	if inString {
		return nil, errors.New("unterminated quoted string")
	}
	return append(parts, s[start:]), nil
}
