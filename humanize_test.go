package humantone

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestHumanizeHappyPath(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "humanize_200.json")},
	})
	c := newTestClient(t, srv.URL())

	res, err := c.Humanize(context.Background(), HumanizeRequest{
		Text:               "Some draft AI text here for testing flow.",
		Level:              LevelStandard,
		OutputFormat:       FormatText,
		CustomInstructions: "be friendly",
	})
	if err != nil {
		t.Fatalf("Humanize: %v", err)
	}
	if res.Text == "" {
		t.Errorf("Text empty (rename from content failed?)")
	}
	if res.OutputFormat != FormatText {
		t.Errorf("OutputFormat = %q, want text", res.OutputFormat)
	}
	if res.CreditsUsed != 3 {
		t.Errorf("CreditsUsed = %d, want 3", res.CreditsUsed)
	}
	if res.RequestID == "" {
		t.Errorf("RequestID empty")
	}

	if got := srv.requests[0].Path; got != "/v1/humanize" {
		t.Errorf("path = %q", got)
	}
	if !strings.Contains(string(srv.requests[0].Body), `"content":"Some draft AI text`) {
		t.Errorf("Text→content rename missing in request body: %s", srv.requests[0].Body)
	}
	if !strings.Contains(string(srv.requests[0].Body), `"custom_instructions":"be friendly"`) {
		t.Errorf("custom_instructions not on wire")
	}
}

func TestHumanizeOmitsCustomInstructionsWhenEmpty(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "humanize_200.json")},
	})
	c := newTestClient(t, srv.URL())
	if _, err := c.Humanize(context.Background(), HumanizeRequest{Text: "x"}); err != nil {
		t.Fatalf("Humanize: %v", err)
	}
	if strings.Contains(string(srv.requests[0].Body), "custom_instructions") {
		t.Errorf("custom_instructions should be omitted when empty: %s", srv.requests[0].Body)
	}
}

func TestHumanizeNotEnoughCredits(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 400, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "error_400_not_enough_credits.json")},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Humanize(context.Background(), HumanizeRequest{Text: "x"})
	if !errors.Is(err, ErrInsufficientCredits) {
		t.Errorf("expected ErrInsufficientCredits, got %v", err)
	}
}

func TestHumanizeInternalServerError(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 500, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "error_500.json")},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Humanize(context.Background(), HumanizeRequest{Text: "x"})
	if !errors.Is(err, ErrAPIError) {
		t.Errorf("expected ErrAPIError, got %v", err)
	}
	// POST default does not retry on 5xx — only one call should land.
	if len(srv.requests) != 1 {
		t.Errorf("expected 1 request, got %d", len(srv.requests))
	}
}

func TestHumanizeCoercionFailureMissingContent(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"},
			Body: []byte(`{"success":true,"output_format":"text","credits_used":1}`)},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Humanize(context.Background(), HumanizeRequest{Text: "x"})
	if !errors.Is(err, ErrAPIError) {
		t.Fatalf("expected ErrAPIError, got %v", err)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As failed")
	}
	if apiErr.Retryable {
		t.Errorf("coercion failure should not be retryable")
	}
	if apiErr.Details["coercion_error"] == nil {
		t.Errorf("expected coercion_error in Details")
	}
}

func TestHumanizeUnknownOutputFormatIsCoercionFailure(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"},
			Body: []byte(`{"success":true,"content":"x","output_format":"csv","credits_used":1}`)},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Humanize(context.Background(), HumanizeRequest{Text: "x"})
	if !errors.Is(err, ErrAPIError) {
		t.Errorf("expected ErrAPIError, got %v", err)
	}
}

func TestHumanizeTextRenameRoundtrip(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"},
			Body: []byte(`{"success":true,"content":"the rewritten body","output_format":"html","credits_used":2,"request_id":"rr"}`)},
	})
	c := newTestClient(t, srv.URL())
	res, err := c.Humanize(context.Background(), HumanizeRequest{Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "the rewritten body" {
		t.Errorf("Text = %q, want rename from content", res.Text)
	}
}

func TestHumanizeRequestIDFromBody(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json", "X-Request-Id": "from-header"},
			Body: []byte(`{"success":true,"content":"x","output_format":"text","credits_used":1,"request_id":"from-body"}`)},
	})
	c := newTestClient(t, srv.URL())
	res, _ := c.Humanize(context.Background(), HumanizeRequest{Text: "x"})
	if res.RequestID != "from-body" {
		t.Errorf("RequestID = %q, want body wins", res.RequestID)
	}
}

func TestHumanizeRequestIDFallsBackToHeader(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json", "X-Request-Id": "from-header"},
			Body: []byte(`{"success":true,"content":"x","output_format":"text","credits_used":1}`)},
	})
	c := newTestClient(t, srv.URL())
	res, _ := c.Humanize(context.Background(), HumanizeRequest{Text: "x"})
	if res.RequestID != "from-header" {
		t.Errorf("RequestID = %q, want from-header", res.RequestID)
	}
}

func TestHumanizeAuthError(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 401, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "error_401_invalid_key.json")},
	})
	c := newTestClient(t, srv.URL())
	_, err := c.Humanize(context.Background(), HumanizeRequest{Text: "x"})
	if !errors.Is(err, ErrAuthentication) {
		t.Errorf("expected ErrAuthentication, got %v", err)
	}
}

// Ensure http method on the wire is POST.
func TestHumanizeMethodIsPOST(t *testing.T) {
	withSilencedSleep(t)
	srv := newRecordingServer(t, []scriptedResponse{
		{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: loadFixture(t, "humanize_200.json")},
	})
	c := newTestClient(t, srv.URL())
	_, _ = c.Humanize(context.Background(), HumanizeRequest{Text: "x"})
	if srv.requests[0].Method != http.MethodPost {
		t.Errorf("method = %q", srv.requests[0].Method)
	}
}
