// Package apperr is the single source of truth for the service's error
// contract: the stable, machine-readable codes returned to clients and the
// typed error that carries them across layers.
//
// Error deliberately knows nothing about HTTP status codes — the policy
// engine stays transport-independent, and the httpapi package owns the
// code-to-status mapping.
package apperr

import (
	"errors"
	"fmt"
)

// Code identifies an error condition in the wire contract.
type Code string

const (
	InvalidJSON      Code = "invalid_json"
	InvalidInput     Code = "invalid_input"
	InvalidPolicy    Code = "invalid_policy"
	InferenceError   Code = "inference_error"
	TooLarge         Code = "request_too_large"
	MethodNotAllowed Code = "method_not_allowed"
	NotFound         Code = "not_found"
	Internal         Code = "internal_error"
)

// Error is the shared coded error. Its JSON shape is the wire error body.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// New builds a coded error with a formatted message.
func New(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// From unwraps err into an *Error, returning nil when err does not carry a
// code. Callers at the boundary decide how to treat uncoded errors.
func From(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}
