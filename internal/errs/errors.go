// Package errs defines the typed error taxonomy used across Appwrite
// Migration Guard. Every error that crosses a package boundary should be
// (or wrap) an *errs.Error so callers can branch on Kind instead of
// matching on error strings.
package errs

import (
	"errors"
	"fmt"
)

// Kind classifies an error into one of a fixed set of categories. Callers
// use Kind to decide whether to retry, how to render the failure, and
// which verification state (PASS/WARN/BLOCK) it maps to.
type Kind string

const (
	KindConnectivity    Kind = "connectivity"     // network/DNS/TLS failure reaching the endpoint
	KindAuthentication  Kind = "authentication"   // credentials rejected (401)
	KindAuthorization   Kind = "authorization"    // credentials valid but lack scope (403)
	KindRateLimit       Kind = "rate_limit"       // request throttled (429)
	KindNotFound        Kind = "not_found"        // resource does not exist (404)
	KindValidation      Kind = "validation"       // request rejected as malformed (400)
	KindPagination      Kind = "pagination"       // pagination cursor/state was inconsistent
	KindTimeout         Kind = "timeout"          // request exceeded its deadline
	KindServer          Kind = "server"           // destination returned a 5xx
	KindInvalidResponse Kind = "invalid_response" // response could not be parsed/understood
	KindComparison      Kind = "comparison"       // manifest comparison could not be completed
	KindConfiguration   Kind = "configuration"    // local configuration is missing/invalid
	KindUnsupported     Kind = "unsupported"      // resource type is explicitly not supported
)

// Error is the concrete error type returned by internal packages.
type Error struct {
	Kind       Kind   // category, used for branching/classification
	Op         string // operation being performed, e.g. "appwrite.ListTables"
	StatusCode int    // HTTP status code, 0 if not applicable
	Retryable  bool   // whether the caller may retry this operation
	Err        error  // wrapped underlying error, may be nil
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("%s: %s", e.Op, e.Kind)
	if e.StatusCode != 0 {
		msg = fmt.Sprintf("%s (http %d)", msg, e.StatusCode)
	}
	if e.Err != nil {
		msg = fmt.Sprintf("%s: %v", msg, e.Err)
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// New constructs an *Error for op, wrapping err (which may be nil).
func New(kind Kind, op string, err error) *Error {
	return &Error{Kind: kind, Op: op, Err: err}
}

// WithStatus sets the HTTP status code and returns the receiver for chaining.
func (e *Error) WithStatus(code int) *Error {
	e.StatusCode = code
	return e
}

// WithRetryable sets whether the error is retryable and returns the receiver.
func (e *Error) WithRetryable(retryable bool) *Error {
	e.Retryable = retryable
	return e
}

// Is reports whether target is an *Error with the same Kind, so callers can
// write errors.Is(err, errs.New(errs.KindNotFound, "", nil)) style checks
// via the helper functions below instead.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	if t.Kind == "" {
		return false
	}
	return e.Kind == t.Kind
}

// KindOf returns the Kind of err if it is (or wraps) an *Error, and ok=false
// otherwise.
func KindOf(err error) (Kind, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind, true
	}
	return "", false
}

// IsKind reports whether err is (or wraps) an *Error of the given kind.
func IsKind(err error, kind Kind) bool {
	k, ok := KindOf(err)
	return ok && k == kind
}

// IsRetryable reports whether err is (or wraps) an *Error marked retryable.
func IsRetryable(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Retryable
	}
	return false
}
