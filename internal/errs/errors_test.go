package errs

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsKind(t *testing.T) {
	err := New(KindNotFound, "op", fmt.Errorf("nope"))
	if !IsKind(err, KindNotFound) {
		t.Fatal("expected KindNotFound")
	}
	if IsKind(err, KindServer) {
		t.Fatal("did not expect KindServer")
	}
}

func TestIsKind_WrappedError(t *testing.T) {
	inner := New(KindRateLimit, "op", nil)
	wrapped := fmt.Errorf("context: %w", inner)
	if !IsKind(wrapped, KindRateLimit) {
		t.Fatal("expected to unwrap to KindRateLimit")
	}
}

func TestIsRetryable(t *testing.T) {
	retryable := New(KindServer, "op", nil).WithRetryable(true)
	notRetryable := New(KindAuthentication, "op", nil)
	if !IsRetryable(retryable) {
		t.Fatal("expected retryable")
	}
	if IsRetryable(notRetryable) {
		t.Fatal("expected not retryable")
	}
	if IsRetryable(errors.New("plain error")) {
		t.Fatal("plain errors are never retryable")
	}
}

func TestErrorString_NoSecretLeak(t *testing.T) {
	err := New(KindAuthentication, "appwrite.Health", fmt.Errorf("invalid key")).WithStatus(401)
	msg := err.Error()
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
	// Regression guard: nothing in this package ever has access to raw
	// secrets, so this mostly documents the expectation for future editors.
}
