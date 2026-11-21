package sqlcockpit

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Codes the client sets on an [*Error] itself. Errors from the portal carry the portal's own
// error.code instead, such as "not_found", "validation_failed" or "rate_limited".
const (
	CodeInvalidConfig      = "invalid_config"
	CodeInvalidArgument    = "invalid_argument"
	CodeUnexpectedRedirect = "unexpected_redirect"
	CodeInvalidResponse    = "invalid_response"
	CodeResponseTooLarge   = "response_too_large"
	CodeConnectionError    = "connection_error"
	CodeTimeout            = "timeout"
)

// Sentinel errors for use with [errors.Is]. An [*Error] matches the sentinel for its HTTP status.
var (
	// ErrUnauthenticated matches a 401: the API key is missing, wrong, revoked or expired.
	ErrUnauthenticated = errors.New("sqlcockpit: unauthenticated")

	// ErrForbidden matches a 403: the key lacks the scope, your plan has no room, or your role
	// cannot do this.
	ErrForbidden = errors.New("sqlcockpit: forbidden")

	// ErrNotFound matches a 404: not found, or not something your account can reach.
	ErrNotFound = errors.New("sqlcockpit: not found")

	// ErrValidation matches a 400 or 422 from the portal, and an id the client refused before
	// sending (code "invalid_argument"). See [Error.Fields].
	ErrValidation = errors.New("sqlcockpit: validation failed")

	// ErrRateLimited matches a 429. See [Error.RetryAfter].
	ErrRateLimited = errors.New("sqlcockpit: rate limited")

	// ErrServer matches a 5xx.
	ErrServer = errors.New("sqlcockpit: server error")
)

// Error is the error the client returns for a failed call.
type Error struct {
	// Code is the portal's error.code (for example "not_found"), or one of the client's own
	// codes: CodeInvalidConfig, CodeInvalidArgument, CodeUnexpectedRedirect,
	// CodeInvalidResponse, CodeResponseTooLarge, CodeConnectionError or CodeTimeout.
	Code string

	// Status is the HTTP status, or 0 when no response was received.
	Status int

	// Message says what went wrong, in plain words.
	Message string

	// Fields holds the messages for each refused field, for validation errors.
	Fields map[string][]string

	// RetryAfter is the wait the portal asked for in its Retry-After header, or 0 when it sent
	// none.
	RetryAfter time.Duration

	err           error
	hasRetryAfter bool
	retry         bool
}

// Error returns the message with the code and HTTP status. It never contains the API key.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("sqlcockpit: ")
	b.WriteString(e.Message)
	switch {
	case e.Status != 0:
		fmt.Fprintf(&b, " (%s, HTTP %d)", e.Code, e.Status)
	case e.Code != "":
		fmt.Fprintf(&b, " (%s)", e.Code)
	}
	return b.String()
}

// Unwrap returns the underlying cause, such as a network error, when there is one.
func (e *Error) Unwrap() error {
	return e.err
}

// Is reports whether the error matches one of the sentinel errors, so that
// errors.Is(err, sqlcockpit.ErrNotFound) works.
func (e *Error) Is(target error) bool {
	switch target {
	case ErrUnauthenticated:
		return e.Status == http.StatusUnauthorized
	case ErrForbidden:
		return e.Status == http.StatusForbidden
	case ErrNotFound:
		return e.Status == http.StatusNotFound
	case ErrValidation:
		return e.Status == http.StatusBadRequest || e.Status == http.StatusUnprocessableEntity || e.Code == CodeInvalidArgument
	case ErrRateLimited:
		return e.Status == http.StatusTooManyRequests
	case ErrServer:
		return e.Status >= 500
	}
	return false
}

// IsUnauthenticated reports whether err is a 401 from the portal.
func IsUnauthenticated(err error) bool { return errors.Is(err, ErrUnauthenticated) }

// IsForbidden reports whether err is a 403 from the portal.
func IsForbidden(err error) bool { return errors.Is(err, ErrForbidden) }

// IsNotFound reports whether err is a 404 from the portal.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsValidation reports whether err is a 400 or 422 from the portal, or an argument the client
// refused before sending.
func IsValidation(err error) bool { return errors.Is(err, ErrValidation) }

// IsRateLimited reports whether err is a 429 from the portal.
func IsRateLimited(err error) bool { return errors.Is(err, ErrRateLimited) }

// IsServerError reports whether err is a 5xx from the portal.
func IsServerError(err error) bool { return errors.Is(err, ErrServer) }

func configError(message string) *Error {
	return &Error{Code: CodeInvalidConfig, Message: message}
}

func notUUIDError(name string) *Error {
	return &Error{
		Code:    CodeInvalidArgument,
		Message: name + " must be a UUID.",
		Fields:  map[string][]string{name: {"Must be a UUID."}},
	}
}
