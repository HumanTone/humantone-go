package humantone

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by SDK methods. Use errors.Is to test for them.
//
// Each *Error returned by the SDK wraps exactly one of these sentinels via
// Unwrap, so errors.Is(err, humantone.ErrAuthentication) (and so on) is the
// idiomatic way to discriminate failures by category. To inspect richer
// context — request ID, HTTP status, retry-after seconds, daily renewal
// window — use errors.As to obtain the *Error value.
var (
	ErrAuthentication      = errors.New("humantone: authentication failed")
	ErrPermission          = errors.New("humantone: permission denied")
	ErrRateLimit           = errors.New("humantone: rate limited")
	ErrInsufficientCredits = errors.New("humantone: insufficient credits")
	ErrDailyLimitExceeded  = errors.New("humantone: daily detection limit reached")
	ErrInvalidRequest      = errors.New("humantone: invalid request")
	ErrNotFound            = errors.New("humantone: not found")
	ErrAPIError            = errors.New("humantone: API error")
	ErrTimeout             = errors.New("humantone: request timed out")
	ErrNetwork             = errors.New("humantone: network error")
	ErrInvalidAPIKey       = errors.New("humantone: invalid API key format")
)

// ErrorCode is a stable string identifier for the kind of error returned by
// the API or detected locally. Constants below cover every value the SDK can
// produce; additional values may appear in the future via the API's planned
// v2 error shape (body.error.code) and will be surfaced verbatim.
type ErrorCode string

// Top-level error codes corresponding to the sentinel errors and to the API's
// canonical error categories.
const (
	ErrCodeAuthentication      ErrorCode = "authentication_error"
	ErrCodePermission          ErrorCode = "permission_denied"
	ErrCodeRateLimit           ErrorCode = "rate_limit"
	ErrCodeInsufficientCredits ErrorCode = "insufficient_credits"
	ErrCodeDailyLimitExceeded  ErrorCode = "daily_limit_exceeded"
	ErrCodeInvalidRequest      ErrorCode = "invalid_request"
	ErrCodeNotFound            ErrorCode = "not_found"
	ErrCodeAPIError            ErrorCode = "api_error"
	ErrCodeTimeout             ErrorCode = "timeout"
	ErrCodeNetwork             ErrorCode = "network_error"
	ErrCodeInvalidAPIKey       ErrorCode = "invalid_api_key_format"
	ErrCodeMissingAPIKey       ErrorCode = "missing_api_key"
)

// Refined error codes that the SDK surfaces for specific recognized message
// patterns on /v1/humanize. These all wrap ErrInvalidRequest as the sentinel
// (or ErrInsufficientCredits for credits) but discriminate the failure mode
// for callers that want finer-grained handling.
const (
	ErrCodeTextTooShort         ErrorCode = "text_too_short"
	ErrCodeTextTooLong          ErrorCode = "text_too_long"
	ErrCodeSafetyCheckFailed    ErrorCode = "safety_check_failed"
	ErrCodeLanguageNotSupported ErrorCode = "language_not_supported"
)

// Error is the rich error type returned by all SDK methods. It always wraps
// exactly one sentinel (accessible via Unwrap and therefore via errors.Is),
// and carries diagnostic context useful for logging, retry decisions, and
// user-facing messages.
//
// Always returned by the SDK as a pointer; obtain it from a returned error
// with errors.As:
//
//	var apiErr *humantone.Error
//	if errors.As(err, &apiErr) {
//	    log.Printf("request_id=%s code=%s", apiErr.RequestID, apiErr.Code)
//	}
type Error struct {
	// Code is a stable machine-readable identifier for the error kind.
	Code ErrorCode

	// StatusCode is the HTTP status returned by the API; 0 for non-HTTP
	// failures such as local validation, network errors, or timeouts.
	StatusCode int

	// Message is the human-readable error description. For API-originated
	// errors it is the verbatim server message; for SDK-originated errors
	// it is the SDK's own description.
	Message string

	// RequestID echoes the API's request_id (from the response body or the
	// X-Request-Id header). Empty string when not provided.
	RequestID string

	// Details holds extra fields supplied by the v2 error shape, plus
	// diagnostic fields for parse and coercion failures (raw_body,
	// parse_error, coercion_error). Nil when there is nothing to report.
	Details map[string]any

	// Retryable indicates whether the underlying condition is, in
	// principle, transient. The SDK retry loop consults this together with
	// the HTTP method and Config.RetryOnPost to decide actual retries.
	Retryable bool

	// RetryAfterSeconds is populated for ErrRateLimit from the Retry-After
	// response header. 0 if the header was absent or unparseable.
	RetryAfterSeconds int

	// TimeToNextRenew is populated for ErrDailyLimitExceeded from the
	// API's time_to_next_renew body field. 0 if the field was omitted by
	// the server.
	TimeToNextRenew int

	// sentinel is the base sentinel error used for errors.Is comparisons.
	sentinel error
}

// Error renders the error as a string suitable for logging.
func (e *Error) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("humantone: %s (code=%s, status=%d, request_id=%s)",
			e.Message, e.Code, e.StatusCode, e.RequestID)
	}
	return fmt.Sprintf("humantone: %s (code=%s, status=%d)",
		e.Message, e.Code, e.StatusCode)
}

// Unwrap returns the sentinel error this *Error wraps, enabling errors.Is to
// walk the chain.
func (e *Error) Unwrap() error { return e.sentinel }

// newError constructs a *Error with the sentinel and code set together. It
// keeps the Sentinel field in sync without exposing it for direct mutation by
// callers in other files.
func newError(sentinel error, code ErrorCode, message string) *Error {
	return &Error{
		Code:     code,
		Message:  message,
		sentinel: sentinel,
	}
}
