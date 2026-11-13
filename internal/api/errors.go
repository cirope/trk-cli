package api

import (
	"errors"
	"net/http"
)

// Errors to test for with errors.Is. An *Error matches the one for its
// status.
var (
	ErrUnauthorized    = errors.New("not signed in, or the session expired or was revoked")
	ErrForbidden       = errors.New("not allowed")
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("conflict")
	ErrUpgradeRequired = errors.New("this version of trk is no longer supported")
	ErrTooLarge        = errors.New("the server's answer is too large")
)

// Error is an error answer from the server: its HTTP status and the
// envelope {"error": {"code", "message", "fields"}}.
type Error struct {
	Status  int
	Code    string              // Stable, snake_case; empty when the body was not the envelope.
	Message string              // For people, shown as given.
	Fields  map[string][]string // Per-field messages on 422.
}

func (e *Error) Error() string { return e.Message }

// Is matches the sentinel for the error's status.
func (e *Error) Is(target error) bool {
	switch e.Status {
	case http.StatusUnauthorized:
		return target == ErrUnauthorized
	case http.StatusForbidden:
		return target == ErrForbidden
	case http.StatusNotFound:
		return target == ErrNotFound
	case http.StatusConflict:
		return target == ErrConflict
	case http.StatusUpgradeRequired:
		return target == ErrUpgradeRequired
	}
	return false
}
