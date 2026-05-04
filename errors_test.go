package humantone

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorStringWithRequestID(t *testing.T) {
	e := newError(ErrAuthentication, ErrCodeAuthentication, "Invalid API key")
	e.StatusCode = 401
	e.RequestID = "req-123"
	got := e.Error()
	want := "humantone: Invalid API key (code=authentication_error, status=401, request_id=req-123)"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrorStringWithoutRequestID(t *testing.T) {
	e := newError(ErrAPIError, ErrCodeAPIError, "Internal server error")
	e.StatusCode = 500
	got := e.Error()
	want := "humantone: Internal server error (code=api_error, status=500)"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrorIsUnwrapsToSentinel(t *testing.T) {
	sentinels := []error{
		ErrAuthentication, ErrPermission, ErrRateLimit,
		ErrInsufficientCredits, ErrDailyLimitExceeded,
		ErrInvalidRequest, ErrNotFound,
		ErrAPIError, ErrTimeout, ErrNetwork, ErrInvalidAPIKey,
	}
	for _, sentinel := range sentinels {
		e := newError(sentinel, ErrCodeAPIError, "test")
		if !errors.Is(e, sentinel) {
			t.Errorf("errors.Is(*Error{sentinel: %v}, %v) = false, want true", sentinel, sentinel)
		}
	}
}

func TestErrorAsExposesFields(t *testing.T) {
	original := newError(ErrRateLimit, ErrCodeRateLimit, "rate limited")
	original.StatusCode = 429
	original.RequestID = "req-456"
	original.RetryAfterSeconds = 30
	original.Retryable = true

	var got *Error
	if !errors.As(error(original), &got) {
		t.Fatalf("errors.As failed to extract *Error")
	}
	if got.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want 429", got.StatusCode)
	}
	if got.RequestID != "req-456" {
		t.Errorf("RequestID = %q, want req-456", got.RequestID)
	}
	if got.RetryAfterSeconds != 30 {
		t.Errorf("RetryAfterSeconds = %d, want 30", got.RetryAfterSeconds)
	}
	if !got.Retryable {
		t.Errorf("Retryable = false, want true")
	}
}

func TestErrorMessageDoesNotLeakAPIKey(t *testing.T) {
	// Defensive: even though the SDK never puts the key into Message,
	// assert that the prefix never appears in a recently-constructed error.
	e := newError(ErrAuthentication, ErrCodeAuthentication, "Invalid API key")
	if strings.Contains(e.Error(), "ht_") {
		t.Errorf("Error() leaked something resembling an API key: %q", e.Error())
	}
}
