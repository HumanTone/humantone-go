package humantone

import (
	"context"
	"errors"
	"testing"
)

func TestDetectHappyPath(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json", "X-Request-Id": "req-d"}, Body: loadFixture(t, "detect_200.json")},
	})
	c := newTestClient(t, srv.URL())
	res, err := c.Detect(context.Background(), "Some text to score for AI likelihood.")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if res.AIScore != 87 {
		t.Errorf("AIScore = %d, want 87", res.AIScore)
	}
	if res.RequestID != "req-d" {
		t.Errorf("RequestID = %q, want req-d", res.RequestID)
	}
}

func TestDetectDailyLimit(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "detect_daily_limit.json")},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Detect(context.Background(), "x")
	if !errors.Is(err, ErrDailyLimitExceeded) {
		t.Fatalf("expected ErrDailyLimitExceeded, got %v", err)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatal("errors.As failed")
	}
	if apiErr.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200 (API quirk)", apiErr.StatusCode)
	}
	if apiErr.TimeToNextRenew != 3600 {
		t.Errorf("TimeToNextRenew = %d, want 3600", apiErr.TimeToNextRenew)
	}
}

func TestDetectServiceErrorRetried(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "detect_service_error.json")},
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "detect_service_error.json")},
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "detect_200.json")},
	})
	c := newTestClient(t, srv.URL())
	res, err := c.Detect(context.Background(), "x")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if res.AIScore != 87 {
		t.Errorf("AIScore = %d", res.AIScore)
	}
	if len(srv.requests) != 3 {
		t.Errorf("expected 3 attempts (2 retries), got %d", len(srv.requests))
	}
}

func TestDetectUnknownErrorMessageNotRetried(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"},
			Body: []byte(`{"success":false,"error":"Some weird detect issue"}`)},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Detect(context.Background(), "x")
	if !errors.Is(err, ErrAPIError) {
		t.Errorf("expected ErrAPIError, got %v", err)
	}
	if len(srv.requests) != 1 {
		t.Errorf("expected 1 attempt (unknown msg = no retry), got %d", len(srv.requests))
	}
	var apiErr *Error
	_ = errors.As(err, &apiErr)
	if apiErr.Message != "Some weird detect issue" {
		t.Errorf("Message = %q, want server msg preserved", apiErr.Message)
	}
}

func TestDetectAIScoreMissing(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"},
			Body: []byte(`{"success":true}`)},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Detect(context.Background(), "x")
	if !errors.Is(err, ErrAPIError) {
		t.Fatalf("expected ErrAPIError (coercion), got %v", err)
	}
}

func TestDetectAIScoreOutOfRange(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"},
			Body: []byte(`{"success":true,"ai_score":150}`)},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Detect(context.Background(), "x")
	if !errors.Is(err, ErrAPIError) {
		t.Fatalf("expected ErrAPIError, got %v", err)
	}
}

func TestDetectAIScoreWrongType(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"},
			Body: []byte(`{"success":true,"ai_score":"87"}`)},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Detect(context.Background(), "x")
	if !errors.Is(err, ErrAPIError) {
		t.Fatalf("expected coercion failure, got %v", err)
	}
}
