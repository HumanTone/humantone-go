package humantone

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestParseResponseSuccess2xx(t *testing.T) {
	body := []byte(`{"success": true, "ai_score": 87}`)
	got, e := parseResponse("POST", "/v1/detect", 200, http.Header{}, body)
	if e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	if string(got) != string(body) {
		t.Errorf("expected raw body returned unchanged")
	}
}

func TestParseResponseSuccessWithoutSuccessField(t *testing.T) {
	// §4.8 1c: success absent → success path (don't validate success==true).
	body := []byte(`{"ai_score": 50}`)
	got, e := parseResponse("POST", "/v1/detect", 200, http.Header{}, body)
	if e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	if string(got) != string(body) {
		t.Errorf("expected raw body returned")
	}
}

func TestParseResponseSuccessFalseAsString(t *testing.T) {
	// §4.8 1b "strict" — only literal JSON `false` triggers error path.
	body := []byte(`{"success": "false", "ai_score": 50}`)
	_, e := parseResponse("POST", "/v1/detect", 200, http.Header{}, body)
	if e != nil {
		t.Fatalf("expected success path, got error: %v", e)
	}
}

func TestParseResponseDailyLimit(t *testing.T) {
	body := []byte(`{"success": false, "error": "Daily usage limit reached. You have used 30 of 30 allowed detections today.", "time_to_next_renew": 3600}`)
	_, e := parseResponse("POST", "/v1/detect", 200, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(e, ErrDailyLimitExceeded) {
		t.Errorf("expected ErrDailyLimitExceeded, got %v", e.sentinel)
	}
	if e.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200 (the API quirk)", e.StatusCode)
	}
	if e.TimeToNextRenew != 3600 {
		t.Errorf("TimeToNextRenew = %d, want 3600", e.TimeToNextRenew)
	}
}

func TestParseResponseDetectServiceErrorRetryable(t *testing.T) {
	body := []byte(`{"success": false}`)
	_, e := parseResponse("POST", "/v1/detect", 200, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(e, ErrAPIError) {
		t.Errorf("expected ErrAPIError, got %v", e.sentinel)
	}
	if !e.Retryable {
		t.Errorf("expected Retryable=true for detect transient error")
	}
}

func TestParseResponseDetectUnknownErrorNotRetryable(t *testing.T) {
	// Per user clarification #3: unknown msg → ErrAPIError non-retryable, server msg preserved.
	body := []byte(`{"success": false, "error": "Some unexpected detection failure"}`)
	_, e := parseResponse("POST", "/v1/detect", 200, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(e, ErrAPIError) {
		t.Errorf("expected ErrAPIError, got %v", e.sentinel)
	}
	if e.Retryable {
		t.Errorf("expected Retryable=false for unknown detect msg")
	}
	if e.Message != "Some unexpected detection failure" {
		t.Errorf("Message = %q, want server message preserved", e.Message)
	}
}

func TestParseResponseHumanizeReservedSuccessFalse(t *testing.T) {
	body := []byte(`{"success": false, "error": "humanize backend hiccup"}`)
	_, e := parseResponse("POST", "/v1/humanize", 200, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(e, ErrAPIError) {
		t.Errorf("expected ErrAPIError")
	}
	if e.Retryable {
		t.Errorf("Retryable should be false at parse stage; retry layer decides")
	}
}

func TestParseResponse4xxKnownError(t *testing.T) {
	body := []byte(`{"error": "Not enough credits"}`)
	_, e := parseResponse("POST", "/v1/humanize", 400, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(e, ErrInsufficientCredits) {
		t.Errorf("expected ErrInsufficientCredits, got %v", e.sentinel)
	}
	if e.Code != ErrCodeInsufficientCredits {
		t.Errorf("Code = %q, want insufficient_credits", e.Code)
	}
	if e.Message != "Not enough credits" {
		t.Errorf("Message = %q, want verbatim", e.Message)
	}
}

func TestParseResponse4xxPatternMatch(t *testing.T) {
	body := []byte(`{"error": "Text exceeds the maximum of 1500 words allowed on your plan"}`)
	_, e := parseResponse("POST", "/v1/humanize", 400, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if e.Code != ErrCodeTextTooLong {
		t.Errorf("Code = %q, want text_too_long", e.Code)
	}
}

func TestParseResponse4xxPrefixMatch(t *testing.T) {
	body := []byte(`{"error": "Your request did not pass the safety check (reason: hate speech)"}`)
	_, e := parseResponse("POST", "/v1/humanize", 400, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if e.Code != ErrCodeSafetyCheckFailed {
		t.Errorf("Code = %q, want safety_check_failed", e.Code)
	}
}

func TestParseResponseUnknown401MapsToAuth(t *testing.T) {
	body := []byte(`{"error": "Unrecognized authentication failure mode"}`)
	_, e := parseResponse("POST", "/v1/humanize", 401, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(e, ErrAuthentication) {
		t.Errorf("expected ErrAuthentication (HTTP-status fallback), got %v", e.sentinel)
	}
}

func TestParseResponseUnknown403MapsToPermission(t *testing.T) {
	body := []byte(`{"error": "some permission issue"}`)
	_, e := parseResponse("POST", "/v1/humanize", 403, http.Header{}, body)
	if !errors.Is(e, ErrPermission) {
		t.Errorf("expected ErrPermission")
	}
}

func TestParseResponseUnknown404MapsToNotFound(t *testing.T) {
	body := []byte(`{"error": "missing resource"}`)
	_, e := parseResponse("GET", "/v1/account", 404, http.Header{}, body)
	if !errors.Is(e, ErrNotFound) {
		t.Errorf("expected ErrNotFound")
	}
}

func TestParseResponse429RateLimit(t *testing.T) {
	body := []byte(`{"error": "Too many requests"}`)
	headers := http.Header{}
	headers.Set("Retry-After", "30")
	_, e := parseResponse("POST", "/v1/humanize", 429, headers, body)
	if !errors.Is(e, ErrRateLimit) {
		t.Errorf("expected ErrRateLimit")
	}
	if e.RetryAfterSeconds != 30 {
		t.Errorf("RetryAfterSeconds = %d, want 30", e.RetryAfterSeconds)
	}
	if !e.Retryable {
		t.Errorf("expected Retryable=true")
	}
}

func TestParseResponse5xxRetryable(t *testing.T) {
	body := []byte(`{"error": "Internal server error"}`)
	_, e := parseResponse("POST", "/v1/humanize", 500, http.Header{}, body)
	if !errors.Is(e, ErrAPIError) {
		t.Errorf("expected ErrAPIError")
	}
	if !e.Retryable {
		t.Errorf("expected Retryable=true on 5xx")
	}
}

func TestParseResponseV2ErrorShape(t *testing.T) {
	body := []byte(`{"error": {"code": "insufficient_credits", "message": "Not enough credits to humanize 1,200 words.", "details": {"required_credits": 12, "available_credits": 4}}, "request_id": "req-v2"}`)
	_, e := parseResponse("POST", "/v1/humanize", 400, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(e, ErrInsufficientCredits) {
		t.Errorf("expected ErrInsufficientCredits, got %v", e.sentinel)
	}
	if e.RequestID != "req-v2" {
		t.Errorf("RequestID = %q, want req-v2", e.RequestID)
	}
	if e.Details["required_credits"] == nil {
		t.Errorf("expected Details to carry required_credits")
	}
}

func TestParseResponseV2UnknownCodeFallsBack(t *testing.T) {
	body := []byte(`{"error": {"code": "weird_new_code", "message": "Something new"}}`)
	_, e := parseResponse("POST", "/v1/humanize", 400, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(e, ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest fallback, got %v", e.sentinel)
	}
	if e.Message != "Something new" {
		t.Errorf("Message = %q, want preserved", e.Message)
	}
}

func TestParseResponseMalformedJSON5xx(t *testing.T) {
	body := []byte(`<html>nginx 502</html>`)
	_, e := parseResponse("GET", "/v1/account", 502, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(e, ErrAPIError) {
		t.Errorf("expected ErrAPIError")
	}
	if !e.Retryable {
		t.Errorf("expected Retryable=true for 5xx parse failure")
	}
	if e.Details["raw_body"] == nil || e.Details["parse_error"] == nil {
		t.Errorf("expected Details to carry raw_body and parse_error")
	}
}

func TestParseResponseMalformedJSON2xxNotRetryable(t *testing.T) {
	body := []byte(`not json`)
	_, e := parseResponse("GET", "/v1/account", 200, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if e.Retryable {
		t.Errorf("expected Retryable=false for 2xx parse failure")
	}
}

func TestRequestIDPrecedenceBodyWinsOverHeader(t *testing.T) {
	body := []byte(`{"error": "Invalid API key", "request_id": "from-body"}`)
	headers := http.Header{}
	headers.Set("X-Request-Id", "from-header")
	_, e := parseResponse("POST", "/v1/humanize", 401, headers, body)
	if e.RequestID != "from-body" {
		t.Errorf("RequestID = %q, want from-body", e.RequestID)
	}
}

func TestRequestIDFallsBackToHeader(t *testing.T) {
	body := []byte(`{"error": "Invalid API key"}`)
	headers := http.Header{}
	headers.Set("X-Request-Id", "from-header")
	_, e := parseResponse("POST", "/v1/humanize", 401, headers, body)
	if e.RequestID != "from-header" {
		t.Errorf("RequestID = %q, want from-header", e.RequestID)
	}
}

func TestRequestIDEmpty(t *testing.T) {
	body := []byte(`{"error": "Invalid API key"}`)
	_, e := parseResponse("POST", "/v1/humanize", 401, http.Header{}, body)
	if e.RequestID != "" {
		t.Errorf("RequestID = %q, want empty", e.RequestID)
	}
}

func TestParseRetryAfterNumeric(t *testing.T) {
	if got := parseRetryAfter("120"); got != 120 {
		t.Errorf("parseRetryAfter(120) = %d", got)
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	// Future-dated value relative to test time. Use far-future to avoid race.
	got := parseRetryAfter("Wed, 21 Oct 2099 07:28:00 GMT")
	if got <= 0 {
		t.Errorf("parseRetryAfter(future date) = %d, want positive", got)
	}
}

func TestParseRetryAfterInvalid(t *testing.T) {
	cases := []string{"", "  ", "garbage", "-5"}
	for _, c := range cases {
		if got := parseRetryAfter(c); got != 0 {
			t.Errorf("parseRetryAfter(%q) = %d, want 0", c, got)
		}
	}
}

func TestParseRetryAfterPastHTTPDate(t *testing.T) {
	got := parseRetryAfter("Wed, 21 Oct 1990 07:28:00 GMT")
	if got != 0 {
		t.Errorf("parseRetryAfter(past) = %d, want 0", got)
	}
}

func TestKnownErrorMessagesAllMatch(t *testing.T) {
	// Spot-check several rows of §4.7 to make sure exact-match semantics hold.
	cases := []struct {
		status   int
		message  string
		sentinel error
		code     ErrorCode
	}{
		{401, "Missing or invalid Authorization header", ErrAuthentication, ErrCodeAuthentication},
		{401, "This API key has been revoked", ErrAuthentication, ErrCodeAuthentication},
		{401, "User not found", ErrAuthentication, ErrCodeAuthentication},
		{403, "Your current plan does not include API access. Please upgrade to continue.", ErrPermission, ErrCodePermission},
		{404, "Not Found", ErrNotFound, ErrCodeNotFound},
		{405, "Method not allowed", ErrInvalidRequest, ErrCodeInvalidRequest},
		{400, "Invalid JSON body", ErrInvalidRequest, ErrCodeInvalidRequest},
		{400, "content is required", ErrInvalidRequest, ErrCodeInvalidRequest},
		{400, "Text must be at least 30 words", ErrInvalidRequest, ErrCodeTextTooShort},
		{400, "humanization_level must be one of: standard, advanced, extreme", ErrInvalidRequest, ErrCodeInvalidRequest},
		{400, "output_format must be one of: html, text, markdown", ErrInvalidRequest, ErrCodeInvalidRequest},
		{400, "custom_instructions must be 1000 characters or fewer", ErrInvalidRequest, ErrCodeInvalidRequest},
		{400, "The advanced and extreme humanization levels are only available for English text", ErrInvalidRequest, ErrCodeLanguageNotSupported},
		{404, "Plan not found", ErrNotFound, ErrCodeNotFound},
		{500, "Internal server error", ErrAPIError, ErrCodeAPIError},
	}
	for _, tc := range cases {
		body := []byte(`{"error": "` + escape(tc.message) + `"}`)
		_, e := parseResponse("POST", "/v1/humanize", tc.status, http.Header{}, body)
		if e == nil {
			t.Errorf("status=%d msg=%q: expected error", tc.status, tc.message)
			continue
		}
		if !errors.Is(e, tc.sentinel) {
			t.Errorf("status=%d msg=%q: sentinel = %v, want %v", tc.status, tc.message, e.sentinel, tc.sentinel)
		}
		if e.Code != tc.code {
			t.Errorf("status=%d msg=%q: code = %q, want %q", tc.status, tc.message, e.Code, tc.code)
		}
		if e.Message != tc.message {
			t.Errorf("status=%d: message = %q, want verbatim %q", tc.status, e.Message, tc.message)
		}
	}
}

// escape is a minimal JSON string escaper for test-fixture construction.
// Sufficient for the ASCII messages in the §4.7 catalog.
func escape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`)
}
