package humantone

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// maxRawBodyBytes caps the amount of raw response body retained in
// *Error.Details on parse or coercion failures. 4 KB is enough for a useful
// snippet without keeping arbitrarily large blobs in memory or logs.
const maxRawBodyBytes = 4096

// matchKind enumerates how an errorRule's pattern is compared against the
// server-supplied message string.
type matchKind int

const (
	matchExact matchKind = iota
	matchPrefix
	matchPattern
)

// errorRule is one row of the §4.7 error catalog: combine an HTTP status with
// a message-matching rule and resolve to a sentinel + code pair.
type errorRule struct {
	status   int
	kind     matchKind
	pattern  string         // literal for exact/prefix; ignored for matchPattern
	regex    *regexp.Regexp // used when kind == matchPattern
	sentinel error
	code     ErrorCode
}

// errorRules is the ordered list of recognized API errors. Order matters only
// within rows that share an HTTP status — the first matching rule wins.
var errorRules = []errorRule{
	// Cross-endpoint authentication errors (HTTP 401).
	{status: 401, kind: matchExact, pattern: "Missing or invalid Authorization header", sentinel: ErrAuthentication, code: ErrCodeAuthentication},
	{status: 401, kind: matchExact, pattern: "Invalid API key format", sentinel: ErrAuthentication, code: ErrCodeAuthentication},
	{status: 401, kind: matchExact, pattern: "Invalid API key", sentinel: ErrAuthentication, code: ErrCodeAuthentication},
	{status: 401, kind: matchExact, pattern: "This API key has been revoked", sentinel: ErrAuthentication, code: ErrCodeAuthentication},
	{status: 401, kind: matchExact, pattern: "User not found", sentinel: ErrAuthentication, code: ErrCodeAuthentication},

	// Cross-endpoint permission and routing.
	{status: 403, kind: matchExact, pattern: "Your current plan does not include API access. Please upgrade to continue.", sentinel: ErrPermission, code: ErrCodePermission},
	{status: 404, kind: matchExact, pattern: "Not Found", sentinel: ErrNotFound, code: ErrCodeNotFound},
	{status: 405, kind: matchExact, pattern: "Method not allowed", sentinel: ErrInvalidRequest, code: ErrCodeInvalidRequest},

	// /v1/humanize errors.
	{status: 400, kind: matchExact, pattern: "Invalid JSON body", sentinel: ErrInvalidRequest, code: ErrCodeInvalidRequest},
	{status: 400, kind: matchExact, pattern: "content is required", sentinel: ErrInvalidRequest, code: ErrCodeInvalidRequest},
	{status: 400, kind: matchExact, pattern: "Text must be at least 30 words", sentinel: ErrInvalidRequest, code: ErrCodeTextTooShort},
	{status: 400, kind: matchPattern, regex: regexp.MustCompile(`^Text exceeds the maximum of \d+ words allowed on your plan$`), sentinel: ErrInvalidRequest, code: ErrCodeTextTooLong},
	{status: 400, kind: matchExact, pattern: "Not enough credits", sentinel: ErrInsufficientCredits, code: ErrCodeInsufficientCredits},
	{status: 400, kind: matchExact, pattern: "humanization_level must be one of: standard, advanced, extreme", sentinel: ErrInvalidRequest, code: ErrCodeInvalidRequest},
	{status: 400, kind: matchExact, pattern: "output_format must be one of: html, text, markdown", sentinel: ErrInvalidRequest, code: ErrCodeInvalidRequest},
	{status: 400, kind: matchExact, pattern: "custom_instructions must be 1000 characters or fewer", sentinel: ErrInvalidRequest, code: ErrCodeInvalidRequest},
	{status: 400, kind: matchExact, pattern: "The advanced and extreme humanization levels are only available for English text", sentinel: ErrInvalidRequest, code: ErrCodeLanguageNotSupported},
	{status: 400, kind: matchPrefix, pattern: "Your request did not pass the safety check", sentinel: ErrInvalidRequest, code: ErrCodeSafetyCheckFailed},
	{status: 404, kind: matchExact, pattern: "Plan not found", sentinel: ErrNotFound, code: ErrCodeNotFound},
	{status: 500, kind: matchExact, pattern: "Internal server error", sentinel: ErrAPIError, code: ErrCodeAPIError},
}

// parseResponse implements the §4.8 error-parsing algorithm. It returns
// either a non-nil success-body slice (the original body, ready for the
// caller to unmarshal further) or a non-nil *Error. Exactly one is non-nil.
//
// method is the HTTP method ("GET", "POST"); path is the URL path (used to
// disambiguate /v1/humanize from /v1/detect for the 200+success:false rules).
func parseResponse(method, path string, status int, headers http.Header, body []byte) ([]byte, *Error) {
	// Step 0: parse JSON. Failure short-circuits to the parse-failure handler.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, parseFailureError(status, headers, body, err)
	}

	requestID := resolveRequestID(raw, headers)

	// Step 1: 2xx responses with success:false body are the API's "soft error" path.
	if status >= 200 && status < 300 {
		if isStrictFalse(raw["success"]) {
			return nil, softErrorFromBody(method, path, status, raw, requestID)
		}
		return body, nil
	}

	// Step 2: 4xx responses — try to match the body.error message.
	if status >= 400 && status < 500 {
		errVal := raw["error"]

		// Try v2 shape first (object with code/message/details).
		if isJSONObject(errVal) {
			return nil, parseV2Error(status, errVal, requestID, headers)
		}

		// Try v1 shape (string).
		var msg string
		if len(errVal) > 0 {
			if err := json.Unmarshal(errVal, &msg); err == nil {
				if rule, ok := matchErrorRule(status, msg); ok {
					return nil, finalizeRuleError(rule, status, msg, requestID, headers)
				}
			}
		}

		// No §4.7 match — fall back to HTTP-status mapping per §4.11.
		return nil, statusFallbackError(status, msg, requestID, headers)
	}

	// Step 4: 5xx — generic transient API error.
	if status >= 500 && status < 600 {
		var msg string
		if errVal := raw["error"]; len(errVal) > 0 {
			_ = json.Unmarshal(errVal, &msg)
		}
		if msg == "" {
			msg = http.StatusText(status)
			if msg == "" {
				msg = fmt.Sprintf("HTTP %d", status)
			}
		}
		e := newError(ErrAPIError, ErrCodeAPIError, msg)
		e.StatusCode = status
		e.RequestID = requestID
		e.Retryable = true
		return nil, e
	}

	// Anything else (1xx, 3xx) — treat as an unexpected API error, not retryable.
	e := newError(ErrAPIError, ErrCodeAPIError, fmt.Sprintf("unexpected HTTP status %d", status))
	e.StatusCode = status
	e.RequestID = requestID
	return nil, e
}

// parseFailureError builds the *Error for §4.8 step 7: JSON parse failure on
// the response body. Retryability depends on HTTP status — only 5xx is
// considered transient.
func parseFailureError(status int, headers http.Header, body []byte, err error) *Error {
	e := newError(ErrAPIError, ErrCodeAPIError, "Failed to parse HumanTone API response as JSON. See exception details.")
	e.StatusCode = status
	e.Retryable = status >= 500 && status < 600
	e.Details = map[string]any{
		"raw_body":    truncate(body, maxRawBodyBytes),
		"parse_error": err.Error(),
	}
	// Body wasn't parseable as JSON, so request_id can only come from header.
	if reqID := headers.Get("X-Request-Id"); reqID != "" {
		e.RequestID = reqID
	}
	return e
}

// softErrorFromBody handles the 2xx + success:false case for both endpoints.
// /v1/detect with the daily-limit message produces ErrDailyLimitExceeded;
// /v1/detect with no message produces a retryable ErrAPIError; /v1/detect
// with an unknown message produces a non-retryable ErrAPIError carrying the
// server's message; /v1/humanize (reserved future case) produces a
// non-retryable ErrAPIError that the retry layer may upgrade based on
// RetryOnPost and method.
func softErrorFromBody(method, path string, status int, raw map[string]json.RawMessage, requestID string) *Error {
	var msg string
	if errVal := raw["error"]; len(errVal) > 0 {
		_ = json.Unmarshal(errVal, &msg)
	}

	// Daily-limit case (currently only on /v1/detect, but match by message
	// regardless of path so unexpected emission elsewhere is still classified).
	if strings.HasPrefix(msg, "Daily usage limit reached") {
		var renew int
		if v := raw["time_to_next_renew"]; len(v) > 0 {
			_ = json.Unmarshal(v, &renew)
		}
		e := newError(ErrDailyLimitExceeded, ErrCodeDailyLimitExceeded, msg)
		e.StatusCode = status
		e.RequestID = requestID
		e.TimeToNextRenew = renew
		return e
	}

	isDetect := method == http.MethodPost && strings.HasSuffix(path, "/v1/detect")
	isHumanize := method == http.MethodPost && strings.HasSuffix(path, "/v1/humanize")

	switch {
	case isDetect && msg == "":
		// Transient backend detection error.
		e := newError(ErrAPIError, ErrCodeAPIError, "detection service returned success:false")
		e.StatusCode = status
		e.RequestID = requestID
		e.Retryable = true
		return e
	case isDetect:
		// Unknown detect error message — surface verbatim, do not retry.
		e := newError(ErrAPIError, ErrCodeAPIError, msg)
		e.StatusCode = status
		e.RequestID = requestID
		e.Retryable = false
		return e
	case isHumanize:
		// Reserved: humanize never returns success:false today. If it does,
		// surface as ErrAPIError. The retry layer may flip Retryable based
		// on RetryOnPost; keep Retryable=false here per the spec note.
		out := msg
		if out == "" {
			out = "humanize service returned success:false"
		}
		e := newError(ErrAPIError, ErrCodeAPIError, out)
		e.StatusCode = status
		e.RequestID = requestID
		e.Retryable = false
		return e
	default:
		out := msg
		if out == "" {
			out = "API returned success:false"
		}
		e := newError(ErrAPIError, ErrCodeAPIError, out)
		e.StatusCode = status
		e.RequestID = requestID
		return e
	}
}

// matchErrorRule walks errorRules looking for the first row whose status
// matches and whose pattern matches msg.
func matchErrorRule(status int, msg string) (errorRule, bool) {
	for _, r := range errorRules {
		if r.status != status {
			continue
		}
		switch r.kind {
		case matchExact:
			if msg == r.pattern {
				return r, true
			}
		case matchPrefix:
			if strings.HasPrefix(msg, r.pattern) {
				return r, true
			}
		case matchPattern:
			if r.regex != nil && r.regex.MatchString(msg) {
				return r, true
			}
		}
	}
	return errorRule{}, false
}

// finalizeRuleError converts a matched rule plus context into the *Error to return.
func finalizeRuleError(rule errorRule, status int, msg, requestID string, headers http.Header) *Error {
	e := newError(rule.sentinel, rule.code, msg)
	e.StatusCode = status
	e.RequestID = requestID
	if rule.sentinel == ErrRateLimit {
		e.RetryAfterSeconds = parseRetryAfter(headers.Get("Retry-After"))
		e.Retryable = true
	}
	return e
}

// statusFallbackError implements the §4.11 HTTP-status fallback for 4xx
// responses whose message did not match any §4.7 rule. The body's error
// string (when present) is preserved as the message.
func statusFallbackError(status int, msg, requestID string, headers http.Header) *Error {
	if msg == "" {
		msg = http.StatusText(status)
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
	}
	switch status {
	case 401:
		e := newError(ErrAuthentication, ErrCodeAuthentication, msg)
		e.StatusCode = status
		e.RequestID = requestID
		return e
	case 403:
		e := newError(ErrPermission, ErrCodePermission, msg)
		e.StatusCode = status
		e.RequestID = requestID
		return e
	case 404:
		e := newError(ErrNotFound, ErrCodeNotFound, msg)
		e.StatusCode = status
		e.RequestID = requestID
		return e
	case 429:
		e := newError(ErrRateLimit, ErrCodeRateLimit, msg)
		e.StatusCode = status
		e.RequestID = requestID
		e.RetryAfterSeconds = parseRetryAfter(headers.Get("Retry-After"))
		e.Retryable = true
		return e
	default:
		e := newError(ErrInvalidRequest, ErrCodeInvalidRequest, msg)
		e.StatusCode = status
		e.RequestID = requestID
		return e
	}
}

// v2ErrorBody mirrors the shape of body.error in the planned v2 envelope.
type v2ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// parseV2Error decodes the v2 structured error body and resolves it to a
// *Error. Unknown codes fall through to the HTTP-status mapping with the
// message preserved.
func parseV2Error(status int, raw json.RawMessage, requestID string, headers http.Header) *Error {
	var v2 v2ErrorBody
	if err := json.Unmarshal(raw, &v2); err != nil {
		// Object that isn't a v2 envelope — fall back on status alone.
		return statusFallbackError(status, "", requestID, headers)
	}

	code, ok, sentinel := lookupCode(v2.Code)
	if !ok {
		// Unknown code — fall back to the status-based mapping while
		// preserving the v2 message and details.
		e := statusFallbackError(status, v2.Message, requestID, headers)
		if len(v2.Details) > 0 {
			e.Details = v2.Details
		}
		return e
	}

	e := newError(sentinel, code, v2.Message)
	e.StatusCode = status
	e.RequestID = requestID
	e.Details = v2.Details
	if sentinel == ErrRateLimit {
		e.RetryAfterSeconds = parseRetryAfter(headers.Get("Retry-After"))
		e.Retryable = true
	}
	if sentinel == ErrAPIError {
		e.Retryable = true
	}
	return e
}

// lookupCode reverse-maps a code string from the v2 error shape to a
// sentinel and a normalized ErrorCode. Returns false if the code is unknown.
func lookupCode(raw string) (ErrorCode, bool, error) {
	switch ErrorCode(raw) {
	case ErrCodeAuthentication:
		return ErrCodeAuthentication, true, ErrAuthentication
	case ErrCodePermission:
		return ErrCodePermission, true, ErrPermission
	case ErrCodeRateLimit:
		return ErrCodeRateLimit, true, ErrRateLimit
	case ErrCodeInsufficientCredits:
		return ErrCodeInsufficientCredits, true, ErrInsufficientCredits
	case ErrCodeDailyLimitExceeded:
		return ErrCodeDailyLimitExceeded, true, ErrDailyLimitExceeded
	case ErrCodeInvalidRequest:
		return ErrCodeInvalidRequest, true, ErrInvalidRequest
	case ErrCodeNotFound:
		return ErrCodeNotFound, true, ErrNotFound
	case ErrCodeAPIError:
		return ErrCodeAPIError, true, ErrAPIError
	case ErrCodeTextTooShort, ErrCodeTextTooLong, ErrCodeSafetyCheckFailed, ErrCodeLanguageNotSupported:
		return ErrorCode(raw), true, ErrInvalidRequest
	}
	return "", false, nil
}

// resolveRequestID applies the §4.14 precedence: body.request_id (if present
// and string-typed) wins, then X-Request-Id header, else empty string.
func resolveRequestID(raw map[string]json.RawMessage, headers http.Header) string {
	if v, ok := raw["request_id"]; ok && len(v) > 0 {
		var s string
		if err := json.Unmarshal(v, &s); err == nil && s != "" {
			return s
		}
	}
	return headers.Get("X-Request-Id")
}

// parseRetryAfter parses the Retry-After header value per RFC 7231 — either
// a delta-seconds integer or an HTTP-date. Returns 0 on any failure or for a
// past HTTP-date.
func parseRetryAfter(h string) int {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if n, err := strconv.Atoi(h); err == nil {
		if n < 0 {
			return 0
		}
		return n
	}
	if t, err := http.ParseTime(h); err == nil {
		delta := int(time.Until(t).Seconds())
		if delta < 0 {
			return 0
		}
		return delta
	}
	return 0
}

// isStrictFalse reports whether raw is the literal JSON value `false`. It
// rejects any other value (true, null, strings, numbers, missing) so the
// soft-error path triggers only on an explicit boolean false per §4.8 1b.
func isStrictFalse(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	return string(raw) == "false"
}

// isJSONObject reports whether raw appears to be a JSON object (ignoring
// surrounding whitespace).
func isJSONObject(raw json.RawMessage) bool {
	for _, b := range raw {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		return b == '{'
	}
	return false
}

// truncate returns the first n bytes of body as a string. Useful for
// embedding response snippets in *Error.Details without unbounded memory use.
func truncate(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return string(body[:n])
}
