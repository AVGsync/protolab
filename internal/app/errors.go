package app

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is an error with an HTTP status and a stable machine code.
// The HTTP layer turns it into {"error": {"code", "message", "details"}}.
type Error struct {
	Status  int
	Code    string
	Message string
	Details any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func notFound(what string) error {
	return &Error{Status: http.StatusNotFound, Code: "not_found", Message: what + " не найден(а)"}
}

func invalid(format string, args ...any) error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_error", Message: fmt.Sprintf(format, args...)}
}

func badRequest(format string, args ...any) error {
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: fmt.Sprintf(format, args...)}
}

func conflict(code, format string, args ...any) error {
	return &Error{Status: http.StatusConflict, Code: code, Message: fmt.Sprintf(format, args...)}
}

// AsError extracts an *Error from err.
func AsError(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
