package humantone

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestLookupCodeAllSentinels exercises every branch of the v2-shape
// reverse-mapping so future code refactors can't silently drop a sentinel.
func TestLookupCodeAllSentinels(t *testing.T) {
	cases := map[ErrorCode]error{
		ErrCodeAuthentication:       ErrAuthentication,
		ErrCodePermission:           ErrPermission,
		ErrCodeRateLimit:            ErrRateLimit,
		ErrCodeInsufficientCredits:  ErrInsufficientCredits,
		ErrCodeDailyLimitExceeded:   ErrDailyLimitExceeded,
		ErrCodeInvalidRequest:       ErrInvalidRequest,
		ErrCodeNotFound:             ErrNotFound,
		ErrCodeAPIError:             ErrAPIError,
		ErrCodeTextTooShort:         ErrInvalidRequest,
		ErrCodeTextTooLong:          ErrInvalidRequest,
		ErrCodeSafetyCheckFailed:    ErrInvalidRequest,
		ErrCodeLanguageNotSupported: ErrInvalidRequest,
	}
	for code, wantSentinel := range cases {
		gotCode, ok, gotSentinel := lookupCode(string(code))
		if !ok {
			t.Errorf("lookupCode(%q) ok=false", code)
			continue
		}
		if gotSentinel != wantSentinel {
			t.Errorf("lookupCode(%q) sentinel = %v, want %v", code, gotSentinel, wantSentinel)
		}
		if gotCode != code {
			t.Errorf("lookupCode(%q) code = %q", code, gotCode)
		}
	}
	if _, ok, _ := lookupCode("definitely_not_a_code"); ok {
		t.Errorf("lookupCode of unknown should return false")
	}
}

// TestClassifyTransportErrorTimeout covers context.DeadlineExceeded.
func TestClassifyTransportErrorTimeout(t *testing.T) {
	e := classifyTransportError(context.DeadlineExceeded)
	if !errors.Is(e, ErrTimeout) {
		t.Errorf("DeadlineExceeded should map to ErrTimeout, got %v", e.sentinel)
	}
}

func TestClassifyTransportErrorCanceled(t *testing.T) {
	e := classifyTransportError(context.Canceled)
	if !errors.Is(e, ErrTimeout) {
		t.Errorf("Canceled should map to ErrTimeout, got %v", e.sentinel)
	}
}

// fakeNetTimeoutErr satisfies net.Error with Timeout()=true.
type fakeNetTimeoutErr struct{}

func (fakeNetTimeoutErr) Error() string   { return "i/o timeout" }
func (fakeNetTimeoutErr) Timeout() bool   { return true }
func (fakeNetTimeoutErr) Temporary() bool { return true }

func TestClassifyTransportErrorNetTimeout(t *testing.T) {
	var ne net.Error = fakeNetTimeoutErr{}
	e := classifyTransportError(ne)
	if !errors.Is(e, ErrTimeout) {
		t.Errorf("net.Error.Timeout=true should map to ErrTimeout, got %v", e.sentinel)
	}
}

func TestClassifyTransportErrorGenericNetwork(t *testing.T) {
	e := classifyTransportError(errors.New("dial tcp: connection refused"))
	if !errors.Is(e, ErrNetwork) {
		t.Errorf("generic transport error should map to ErrNetwork, got %v", e.sentinel)
	}
	if !e.Retryable {
		t.Errorf("network errors should be Retryable")
	}
}

// TestNetworkErrorRetriedOnGET exercises the real transport-error path of
// http.go by pointing the client at a closed listener (instant connection
// refused) and verifying the retry layer attempts again.
func TestNetworkErrorRetriedOnGET(t *testing.T) {
	withSilencedSleep(t)

	// Reserve and immediately close a port to guarantee connection refused.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	c, err := NewClient(Config{APIKey: validTestAPIKey, BaseURL: "http://" + addr, MaxRetries: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Account.Get(context.Background())
	if !errors.Is(err, ErrNetwork) {
		t.Errorf("expected ErrNetwork, got %v", err)
	}
}

// TestParseResponseV2RateLimitWithRetryAfter covers the rate-limit branch
// inside parseV2Error.
func TestParseResponseV2RateLimitWithRetryAfter(t *testing.T) {
	body := []byte(`{"error":{"code":"rate_limit","message":"slow down"}}`)
	headers := http.Header{}
	headers.Set("Retry-After", "5")
	_, e := parseResponse("POST", "/v1/humanize", 429, headers, body)
	if !errors.Is(e, ErrRateLimit) {
		t.Errorf("expected ErrRateLimit")
	}
	if e.RetryAfterSeconds != 5 {
		t.Errorf("RetryAfterSeconds = %d, want 5", e.RetryAfterSeconds)
	}
}

func TestParseResponseV2APIErrorRetryable(t *testing.T) {
	body := []byte(`{"error":{"code":"api_error","message":"oops"}}`)
	_, e := parseResponse("GET", "/v1/account", 500, http.Header{}, body)
	if !errors.Is(e, ErrAPIError) {
		t.Errorf("expected ErrAPIError")
	}
	if !e.Retryable {
		t.Errorf("v2 api_error should be Retryable")
	}
}

func TestParseResponseV2MalformedBody(t *testing.T) {
	// body.error is an object but not a v2 envelope (missing fields, wrong types)
	body := []byte(`{"error": {"code": 123}}`)
	_, e := parseResponse("POST", "/v1/humanize", 400, http.Header{}, body)
	if e == nil {
		t.Fatal("expected error")
	}
	// Should fall back to status mapping (400 → invalid_request).
	if !errors.Is(e, ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest fallback, got %v", e.sentinel)
	}
}

func TestIsJSONObjectVariousInputs(t *testing.T) {
	cases := map[string]bool{
		`{`:        true,
		`  {  `:    true,
		`{"a":1}`:  true,
		`[1,2]`:    false,
		`"string"`: false,
		`null`:     false,
		``:         false,
		`   `:      false,
		"\t\n  {":  true,
	}
	for input, want := range cases {
		if got := isJSONObject(json.RawMessage(input)); got != want {
			t.Errorf("isJSONObject(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestTruncateLargeBody(t *testing.T) {
	big := make([]byte, maxRawBodyBytes*2)
	for i := range big {
		big[i] = 'a'
	}
	got := truncate(big, maxRawBodyBytes)
	if len(got) != maxRawBodyBytes {
		t.Errorf("truncated length = %d, want %d", len(got), maxRawBodyBytes)
	}
}

func TestStatusFallbackUnknownStatusEmpty(t *testing.T) {
	// Unknown 4xx with empty body produces ErrInvalidRequest with a
	// generated message. Covers the http.StatusText "" fallback branch.
	_, e := parseResponse("POST", "/v1/humanize", 418, http.Header{}, []byte(`{}`))
	if !errors.Is(e, ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest")
	}
	if e.Message == "" {
		t.Errorf("Message should be filled with HTTP description")
	}
}

func TestParseResponseUnexpected3xx(t *testing.T) {
	_, e := parseResponse("GET", "/v1/account", 301, http.Header{}, []byte(`{}`))
	if !errors.Is(e, ErrAPIError) {
		t.Errorf("3xx should map to ErrAPIError, got %v", e.sentinel)
	}
}

func TestParseResponse5xxNoMessageInBody(t *testing.T) {
	_, e := parseResponse("GET", "/v1/account", 503, http.Header{}, []byte(`{}`))
	if !errors.Is(e, ErrAPIError) {
		t.Errorf("expected ErrAPIError")
	}
	if e.Message == "" {
		t.Errorf("Message should be filled by status text fallback")
	}
}

func TestSoftErrorOnGET(t *testing.T) {
	// 200+success:false on GET (unusual but possible) covers the default
	// branch in softErrorFromBody.
	body := []byte(`{"success":false,"error":"weird account state"}`)
	_, e := parseResponse("GET", "/v1/account", 200, http.Header{}, body)
	if !errors.Is(e, ErrAPIError) {
		t.Errorf("expected ErrAPIError")
	}
	if e.Message != "weird account state" {
		t.Errorf("Message should preserve server text")
	}
}

func TestSoftErrorOnGETNoMessage(t *testing.T) {
	body := []byte(`{"success":false}`)
	_, e := parseResponse("GET", "/v1/account", 200, http.Header{}, body)
	if !errors.Is(e, ErrAPIError) {
		t.Errorf("expected ErrAPIError")
	}
	if e.Message == "" {
		t.Errorf("Message should have a fallback")
	}
}

func TestSoftErrorHumanizeNoMessage(t *testing.T) {
	body := []byte(`{"success":false}`)
	_, e := parseResponse("POST", "/v1/humanize", 200, http.Header{}, body)
	if !errors.Is(e, ErrAPIError) {
		t.Errorf("expected ErrAPIError")
	}
}

func TestParseFailureWithRequestIDHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Request-Id", "req-pf")
	_, e := parseResponse("GET", "/v1/account", 502, headers, []byte("not json"))
	if e.RequestID != "req-pf" {
		t.Errorf("RequestID = %q, want req-pf", e.RequestID)
	}
}

func TestSDKVersionResolutionFallback(t *testing.T) {
	// In the test binary, runtime/debug.ReadBuildInfo does not list this
	// module as a dep — so resolveSDKVersion must return the constant.
	got := resolveSDKVersion()
	if got == "" {
		t.Errorf("resolveSDKVersion returned empty")
	}
	// Either constant (development) or a real semver — both shapes are OK.
	_ = got
}

// TestExecuteWithRetryNonRetryableError exercises the early-return branch
// when shouldRetry returns false on the first attempt.
func TestExecuteWithRetryNonRetryableError(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 403, Body: []byte(`{"error":"Your current plan does not include API access. Please upgrade to continue."}`), Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Account.Get(context.Background())
	if !errors.Is(err, ErrPermission) {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}

// TestSleepFuncImmediateZero covers the d<=0 short-circuit.
func TestSleepFuncImmediateZero(t *testing.T) {
	orig := sleepFunc
	t.Cleanup(func() { sleepFunc = orig })

	// Use the production sleepFunc (don't silence it).
	if err := orig(context.Background(), 0); err != nil {
		t.Errorf("sleep(0) returned error %v", err)
	}
	if err := orig(context.Background(), -1*time.Second); err != nil {
		t.Errorf("sleep(negative) returned error %v", err)
	}
}

// TestSleepFuncCancellation verifies real cancellation interrupts the timer.
func TestSleepFuncCancellation(t *testing.T) {
	orig := sleepFunc
	t.Cleanup(func() { sleepFunc = orig })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := orig(ctx, 5*time.Second)
	elapsed := time.Since(start)
	if err == nil {
		t.Errorf("expected cancellation error")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("cancellation slow: %v", elapsed)
	}
}
