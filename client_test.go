package humantone

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClientMissingKey(t *testing.T) {
	t.Setenv("HUMANTONE_API_KEY", "")
	_, err := NewClient(Config{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidAPIKey) {
		t.Errorf("expected ErrInvalidAPIKey, got %v", err)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As failed")
	}
	if apiErr.Code != ErrCodeMissingAPIKey {
		t.Errorf("Code = %q, want missing_api_key", apiErr.Code)
	}
}

func TestNewClientEmptyEnvKeyTreatedAsUnset(t *testing.T) {
	t.Setenv("HUMANTONE_API_KEY", "")
	_, err := NewClient(Config{})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != ErrCodeMissingAPIKey {
		t.Errorf("empty env should produce missing_api_key, got %v", err)
	}
}

func TestNewClientWhitespaceEnvKeyTreatedAsUnset(t *testing.T) {
	t.Setenv("HUMANTONE_API_KEY", "   ")
	_, err := NewClient(Config{})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != ErrCodeMissingAPIKey {
		t.Errorf("whitespace env should produce missing_api_key, got %v", err)
	}
}

func TestNewClientWhitespacePaddedKeyAccepted(t *testing.T) {
	t.Setenv("HUMANTONE_API_KEY", "")
	c, err := NewClient(Config{APIKey: "  " + validTestAPIKey + "  "})
	if err != nil {
		t.Fatalf("expected acceptance, got %v", err)
	}
	if c.cfg.APIKey != validTestAPIKey {
		t.Errorf("APIKey not trimmed: %q", c.cfg.APIKey)
	}
}

func TestNewClientMalformedKey(t *testing.T) {
	t.Setenv("HUMANTONE_API_KEY", "")
	_, err := NewClient(Config{APIKey: "not-a-valid-key"})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != ErrCodeInvalidAPIKey {
		t.Errorf("malformed key should produce invalid_api_key_format, got %v", err)
	}
}

func TestNewClientEnvFallback(t *testing.T) {
	t.Setenv("HUMANTONE_API_KEY", validTestAPIKey)
	c, err := NewClient(Config{})
	if err != nil {
		t.Fatalf("expected acceptance, got %v", err)
	}
	if c.cfg.APIKey != validTestAPIKey {
		t.Errorf("APIKey not picked up from env")
	}
}

func TestNewClientConfigOverridesEnv(t *testing.T) {
	other := "ht_ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	t.Setenv("HUMANTONE_API_KEY", other)
	c, err := NewClient(Config{APIKey: validTestAPIKey})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if c.cfg.APIKey != validTestAPIKey {
		t.Errorf("Config.APIKey should win, got %q", c.cfg.APIKey)
	}
}

func TestNewClientBaseURLPrecedence(t *testing.T) {
	t.Setenv("HUMANTONE_BASE_URL", "https://env.example.com")
	c, err := NewClient(Config{APIKey: validTestAPIKey, BaseURL: "https://config.example.com"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if c.baseURL != "https://config.example.com" {
		t.Errorf("BaseURL = %q, want config value", c.baseURL)
	}
}

func TestNewClientBaseURLEnvFallback(t *testing.T) {
	t.Setenv("HUMANTONE_BASE_URL", "https://env.example.com")
	c, err := NewClient(Config{APIKey: validTestAPIKey})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if c.baseURL != "https://env.example.com" {
		t.Errorf("BaseURL = %q, want env value", c.baseURL)
	}
}

func TestNewClientBaseURLEmptyEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("HUMANTONE_BASE_URL", "")
	c, err := NewClient(Config{APIKey: validTestAPIKey})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if c.baseURL != defaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", c.baseURL, defaultBaseURL)
	}
}

func TestNewClientZeroValueDefaults(t *testing.T) {
	c, err := NewClient(Config{APIKey: validTestAPIKey})
	if err != nil {
		t.Fatal(err)
	}
	if c.cfg.Timeout != defaultTimeout {
		t.Errorf("Timeout = %v, want %v", c.cfg.Timeout, defaultTimeout)
	}
	if c.cfg.MaxRetries != defaultMaxRetries {
		t.Errorf("MaxRetries = %d, want %d", c.cfg.MaxRetries, defaultMaxRetries)
	}
}

func TestNewClientCustomHTTPClientNotOverridden(t *testing.T) {
	custom := &http.Client{Timeout: 7 * time.Second}
	c, err := NewClient(Config{APIKey: validTestAPIKey, HTTPClient: custom, Timeout: 99 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if c.httpClient != custom {
		t.Errorf("expected injected client to be reused")
	}
	if custom.Timeout != 7*time.Second {
		t.Errorf("injected client's Timeout was modified to %v", custom.Timeout)
	}
}

func TestNewClientAccountResourceWired(t *testing.T) {
	c, err := NewClient(Config{APIKey: validTestAPIKey})
	if err != nil {
		t.Fatal(err)
	}
	if c.Account == nil {
		t.Fatal("Account is nil")
	}
	if c.Account.client != c {
		t.Errorf("Account.client not back-pointing to *Client")
	}
}

// TestHumanizeRequestDefaultsAppliedOnWire asserts that the SDK fills in
// Level=standard and OutputFormat=text on the outgoing JSON body when the
// caller leaves them at zero value.
func TestHumanizeRequestDefaultsAppliedOnWire(t *testing.T) {
	withDeterministicJitter(t)
	withSilencedSleep(t)

	var captured map[string]any
	handler := func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"request_id":"r","content":"x","output_format":"text","credits_used":1}`))
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	_, err := c.Humanize(context.Background(), HumanizeRequest{Text: "anything"})
	if err != nil {
		t.Fatalf("Humanize: %v", err)
	}

	if captured["humanization_level"] != "standard" {
		t.Errorf("humanization_level = %v, want standard", captured["humanization_level"])
	}
	if captured["output_format"] != "text" {
		t.Errorf("output_format = %v, want text (SDK default)", captured["output_format"])
	}
}

func TestRequestSetsHeaders(t *testing.T) {
	withSilencedSleep(t)
	var hdr http.Header
	handler := func(w http.ResponseWriter, r *http.Request) {
		hdr = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"plan":{"id":"p","name":"P","max_words":1,"monthly_credits":1,"api_access":true},"credits":{"trial":0,"subscription":0,"extra":0,"total":0},"subscription":{"active":false}}`))
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	if _, err := c.Account.Get(context.Background()); err != nil {
		t.Fatalf("Account.Get: %v", err)
	}

	if got := hdr.Get("Authorization"); got != "Bearer "+validTestAPIKey {
		t.Errorf("Authorization = %q", got)
	}
	if got := hdr.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
	if got := hdr.Get("Content-Type"); got != "" {
		t.Errorf("GET should not send Content-Type, got %q", got)
	}
	if got := hdr.Get("User-Agent"); got == "" {
		t.Errorf("User-Agent missing")
	}
}

func TestPOSTSetsContentType(t *testing.T) {
	withSilencedSleep(t)
	var hdr http.Header
	handler := func(w http.ResponseWriter, r *http.Request) {
		hdr = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"ai_score":50}`))
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	if _, err := c.Detect(context.Background(), "x"); err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got := hdr.Get("Content-Type"); got != "application/json" {
		t.Errorf("POST Content-Type = %q, want application/json", got)
	}
}
