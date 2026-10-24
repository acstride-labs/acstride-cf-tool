// Package core provides Codeforces operations without terminal output or global state.
package core

import (
	"context"
	"errors"
	"net"
)

type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (e *Error) Error() string        { return e.Message }
func Fail(code, message string) error { return &Error{Code: code, Message: message} }
func Classify(err error) *Error {
	var known *Error
	if errors.As(err, &known) {
		return known
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Code: "cancelled", Message: "Operation cancelled."}
	}
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return &Error{Code: "timeout", Message: "Request timed out.", Retryable: true}
	}
	// Do not expose URLs, cookies, passwords, source or raw server HTML in errors.
	return &Error{Code: "request_failed", Message: "Operation failed. Check connectivity and local file permissions."}
}
