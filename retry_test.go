package humantone

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// healthyAccount200 is the canonical valid /v1/account body for retry tests.
var healthyAccount200 = []byte(`{"plan":{"id":"p","name":"P","max_words":1,"monthly_credits":1,"api_access":true},"credits":{"trial":0,"subscription":0,"extra":0,"total":0},"subscription":{"active":false}}`)

func TestRetryGET5xxThenSuccess(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 500, Body: []byte(`{"error":"Internal server error"}`), Headers: map[string]string{"Content-Type": "application/json"}},
		{Status: 500, Body: []byte(`{"error":"Internal server error"}`), Headers: map[string]string{"Content-Type": "application/json"}},
		{Status: 200, Body: healthyAccount200, Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c := newTestClient(t, srv.URL())
	if _, err := c.Account.Get(context.Background()); err != nil {
		t.Fatalf("Account.Get: %v", err)
	}
	if len(srv.requests) != 3 {
		t.Errorf("expected 3 attempts, got %d", len(srv.requests))
	}
}

func TestRetryGET5xxExhausted(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 500, Body: []byte(`{"error":"Internal server error"}`), Headers: map[string]string{"Content-Type": "application/json"}},
		{Status: 500, Body: []byte(`{"error":"Internal server error"}`), Headers: map[string]string{"Content-Type": "application/json"}},
		{Status: 500, Body: []byte(`{"error":"Internal server error"}`), Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Account.Get(context.Background())
	if !errors.Is(err, ErrAPIError) {
		t.Errorf("expected ErrAPIError, got %v", err)
	}
	if len(srv.requests) != 3 {
		t.Errorf("expected 3 attempts (1 + 2 retries), got %d", len(srv.requests))
	}
}

func TestRetryPOSTHumanize5xxNoRetryByDefault(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 500, Body: []byte(`{"error":"Internal server error"}`), Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Humanize(context.Background(), HumanizeRequest{Text: "x"})
	if !errors.Is(err, ErrAPIError) {
		t.Errorf("expected ErrAPIError, got %v", err)
	}
	if len(srv.requests) != 1 {
		t.Errorf("expected 1 attempt (POST no retry), got %d", len(srv.requests))
	}
}

func TestRetryPOSTHumanize5xxWithRetryOnPost(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 500, Body: []byte(`{"error":"Internal server error"}`), Headers: map[string]string{"Content-Type": "application/json"}},
		{Status: 500, Body: []byte(`{"error":"Internal server error"}`), Headers: map[string]string{"Content-Type": "application/json"}},
		{Status: 200, Body: []byte(`{"success":true,"content":"x","output_format":"text","credits_used":1,"request_id":"r"}`), Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c, err := NewClient(Config{APIKey: validTestAPIKey, BaseURL: srv.URL(), MaxRetries: 2, RetryOnPost: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Humanize(context.Background(), HumanizeRequest{Text: "x"}); err != nil {
		t.Fatalf("Humanize: %v", err)
	}
	if len(srv.requests) != 3 {
		t.Errorf("expected 3 attempts, got %d", len(srv.requests))
	}
}

func TestRetry429AlwaysRetriesOnPOST(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 429, Body: []byte(`{"error":"rate"}`), Headers: map[string]string{"Content-Type": "application/json", "Retry-After": "0"}},
		{Status: 429, Body: []byte(`{"error":"rate"}`), Headers: map[string]string{"Content-Type": "application/json", "Retry-After": "0"}},
		{Status: 200, Body: []byte(`{"success":true,"ai_score":50}`), Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c := newTestClient(t, srv.URL())
	if _, err := c.Detect(context.Background(), "x"); err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(srv.requests) != 3 {
		t.Errorf("expected 3 attempts (429 always retries), got %d", len(srv.requests))
	}
}

func TestRetry4xxNotRetried(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 400, Body: []byte(`{"error":"Invalid JSON body"}`), Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Account.Get(context.Background())
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest, got %v", err)
	}
	if len(srv.requests) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(srv.requests))
	}
}

func TestRetryAuthErrorNotRetried(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 401, Body: []byte(`{"error":"Invalid API key"}`), Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Account.Get(context.Background())
	if !errors.Is(err, ErrAuthentication) {
		t.Errorf("expected ErrAuthentication, got %v", err)
	}
	if len(srv.requests) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(srv.requests))
	}
}

func TestRetryAfterNumericHonored(t *testing.T) {
	withDeterministicJitter(t)
	var sleeps []time.Duration
	orig := sleepFunc
	sleepFunc = func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}
	t.Cleanup(func() { sleepFunc = orig })

	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 429, Body: []byte(`{"error":"rate"}`), Headers: map[string]string{"Content-Type": "application/json", "Retry-After": "3"}},
		{Status: 200, Body: []byte(`{"success":true,"ai_score":50}`), Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c := newTestClient(t, srv.URL())
	if _, err := c.Detect(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if len(sleeps) != 1 {
		t.Fatalf("expected 1 sleep, got %d", len(sleeps))
	}
	if sleeps[0] != 3*time.Second {
		t.Errorf("sleep = %v, want 3s", sleeps[0])
	}
}

func TestRetryAfterHTTPDateHonored(t *testing.T) {
	withDeterministicJitter(t)
	var sleeps []time.Duration
	orig := sleepFunc
	sleepFunc = func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}
	t.Cleanup(func() { sleepFunc = orig })

	future := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 429, Body: []byte(`{"error":"rate"}`), Headers: map[string]string{"Content-Type": "application/json", "Retry-After": future}},
		{Status: 200, Body: []byte(`{"success":true,"ai_score":50}`), Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c := newTestClient(t, srv.URL())
	if _, err := c.Detect(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if len(sleeps) != 1 {
		t.Fatalf("expected 1 sleep, got %d", len(sleeps))
	}
	if sleeps[0] < 1*time.Second || sleeps[0] > 3*time.Second {
		t.Errorf("sleep = %v, expected ~2s", sleeps[0])
	}
}

func TestRetryContextCancelledMidBackoff(t *testing.T) {
	// Sleep with real timer so cancellation has a chance to abort.
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 500, Body: []byte(`{"error":"Internal server error"}`), Headers: map[string]string{"Content-Type": "application/json"}},
		{Status: 500, Body: []byte(`{"error":"Internal server error"}`), Headers: map[string]string{"Content-Type": "application/json"}},
		{Status: 500, Body: []byte(`{"error":"Internal server error"}`), Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c := newTestClient(t, srv.URL())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.Account.Get(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	// Should bail out well before 3 full retries (~500ms+1000ms = 1.5s).
	if elapsed > 800*time.Millisecond {
		t.Errorf("took too long (%v) — expected prompt cancel", elapsed)
	}
}

func TestRetryNetworkErrorDecisions(t *testing.T) {
	// Direct shouldRetry table test — true network failures are awkward to
	// script via httptest, but the policy decision is what we need to lock in.
	netErr := newError(ErrNetwork, ErrCodeNetwork, "dial tcp: connection refused")
	netErr.Retryable = true

	cfg := Config{MaxRetries: 2}
	if !shouldRetry("GET", "/v1/account", netErr, cfg, 0) {
		t.Errorf("GET network error attempt 0 should retry")
	}
	if !shouldRetry("GET", "/v1/account", netErr, cfg, 1) {
		t.Errorf("GET network error attempt 1 should retry")
	}
	if shouldRetry("GET", "/v1/account", netErr, cfg, 2) {
		t.Errorf("GET network error attempt 2 should NOT retry (exhausted)")
	}
	if shouldRetry("POST", "/v1/humanize", netErr, cfg, 0) {
		t.Errorf("POST network error should not retry by default")
	}
	cfg.RetryOnPost = true
	if !shouldRetry("POST", "/v1/humanize", netErr, cfg, 0) {
		t.Errorf("POST network error should retry with RetryOnPost=true")
	}
}

func TestRetry200SuccessFalseDetectRetried(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Body: []byte(`{"success":false}`), Headers: map[string]string{"Content-Type": "application/json"}},
		{Status: 200, Body: []byte(`{"success":true,"ai_score":50}`), Headers: map[string]string{"Content-Type": "application/json"}},
	})
	c := newTestClient(t, srv.URL())
	if _, err := c.Detect(context.Background(), "x"); err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(srv.requests) != 2 {
		t.Errorf("expected 2 attempts, got %d", len(srv.requests))
	}
}

func TestBackoffDurationExponentialGrowth(t *testing.T) {
	withDeterministicJitter(t)
	netErr := newError(ErrNetwork, ErrCodeNetwork, "x")
	netErr.Retryable = true

	d0 := backoffDuration(0, netErr)
	d1 := backoffDuration(1, netErr)
	d2 := backoffDuration(2, netErr)
	if d0 != 500*time.Millisecond {
		t.Errorf("attempt 0: %v", d0)
	}
	if d1 != 1*time.Second {
		t.Errorf("attempt 1: %v", d1)
	}
	if d2 != 2*time.Second {
		t.Errorf("attempt 2: %v", d2)
	}
}

func TestBackoffRespects429RetryAfter(t *testing.T) {
	withDeterministicJitter(t)
	rateErr := newError(ErrRateLimit, ErrCodeRateLimit, "x")
	rateErr.RetryAfterSeconds = 7
	rateErr.Retryable = true

	if got := backoffDuration(0, rateErr); got != 7*time.Second {
		t.Errorf("backoff = %v, want 7s", got)
	}
}

func TestBackoff429NoRetryAfterUsesExponential(t *testing.T) {
	withDeterministicJitter(t)
	rateErr := newError(ErrRateLimit, ErrCodeRateLimit, "x")
	rateErr.Retryable = true

	if got := backoffDuration(0, rateErr); got != 1*time.Second {
		t.Errorf("backoff = %v, want 1s", got)
	}
	if got := backoffDuration(1, rateErr); got != 2*time.Second {
		t.Errorf("backoff = %v, want 2s", got)
	}
}
