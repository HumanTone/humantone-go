package humantone

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAccountGetHappyPath(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "account_200.json")},
	})
	c := newTestClient(t, srv.URL())

	info, err := c.Account.Get(context.Background())
	if err != nil {
		t.Fatalf("Account.Get: %v", err)
	}
	if info.Plan.ID != "pro_monthly" {
		t.Errorf("Plan.ID = %q", info.Plan.ID)
	}
	if info.Plan.MaxWords != 1500 {
		t.Errorf("Plan.MaxWords = %d", info.Plan.MaxWords)
	}
	if info.Credits.Total != 970 {
		t.Errorf("Credits.Total = %d", info.Credits.Total)
	}
	if info.Subscription.ExpiresAt == nil {
		t.Fatalf("ExpiresAt is nil")
	}
	want := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	if !info.Subscription.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", info.Subscription.ExpiresAt, want)
	}
}

func TestAccountExpiresAtAbsent(t *testing.T) {
	withSilencedSleep(t)
	body := []byte(`{"plan":{"id":"p","name":"P","max_words":1,"monthly_credits":1,"api_access":true},"credits":{"trial":0,"subscription":0,"extra":0,"total":0},"subscription":{"active":false}}`)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: body},
	})
	c := newTestClient(t, srv.URL())
	info, err := c.Account.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Subscription.ExpiresAt != nil {
		t.Errorf("ExpiresAt should be nil when absent, got %v", info.Subscription.ExpiresAt)
	}
}

func TestAccountExpiresAtNull(t *testing.T) {
	withSilencedSleep(t)
	body := []byte(`{"plan":{"id":"p","name":"P","max_words":1,"monthly_credits":1,"api_access":true},"credits":{"trial":0,"subscription":0,"extra":0,"total":0},"subscription":{"active":false,"expires_at":null}}`)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: body},
	})
	c := newTestClient(t, srv.URL())
	info, err := c.Account.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Subscription.ExpiresAt != nil {
		t.Errorf("ExpiresAt should be nil for null, got %v", info.Subscription.ExpiresAt)
	}
}

func TestAccountExpiresAtInvalidIsCoercionFailure(t *testing.T) {
	withSilencedSleep(t)
	body := []byte(`{"plan":{"id":"p","name":"P","max_words":1,"monthly_credits":1,"api_access":true},"credits":{"trial":0,"subscription":0,"extra":0,"total":0},"subscription":{"active":false,"expires_at":"not a date"}}`)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: body},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Account.Get(context.Background())
	if !errors.Is(err, ErrAPIError) {
		t.Errorf("expected ErrAPIError, got %v", err)
	}
}

func TestAccountMissingPlanIsCoercionFailure(t *testing.T) {
	withSilencedSleep(t)
	body := []byte(`{"credits":{"trial":0,"subscription":0,"extra":0,"total":0},"subscription":{"active":false}}`)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: body},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Account.Get(context.Background())
	if !errors.Is(err, ErrAPIError) {
		t.Errorf("expected ErrAPIError, got %v", err)
	}
}

func TestAccountMissingCreditsField(t *testing.T) {
	withSilencedSleep(t)
	body := []byte(`{"plan":{"id":"p","name":"P","max_words":1,"monthly_credits":1,"api_access":true},"credits":{"trial":0,"subscription":0,"extra":0},"subscription":{"active":false}}`)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: body},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Account.Get(context.Background())
	if !errors.Is(err, ErrAPIError) {
		t.Errorf("expected ErrAPIError, got %v", err)
	}
}

func TestAccountGetMethodIsGET(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "account_200.json")},
	})
	c := newTestClient(t, srv.URL())
	if _, err := c.Account.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if srv.requests[0].Method != "GET" {
		t.Errorf("method = %q", srv.requests[0].Method)
	}
}

func TestAccountRequestIDFromHeader(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json", "X-Request-Id": "req-acc"}, Body: loadFixture(t, "account_200.json")},
	})
	c := newTestClient(t, srv.URL())
	info, err := c.Account.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.RequestID != "req-acc" {
		t.Errorf("RequestID = %q, want req-acc", info.RequestID)
	}
}
