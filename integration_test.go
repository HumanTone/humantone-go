//go:build integration
// +build integration

// Integration tests against the live HumanTone API. These never run under
// `go test ./...` — they require the `integration` build tag and a real API
// key in HUMANTONE_TEST_API_KEY. Without the key, TestMain exits cleanly
// with status 0 so CI does not flag a missing-key as a failure.

package humantone_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/humantone/humantone-go"
)

func TestMain(m *testing.M) {
	if strings.TrimSpace(os.Getenv("HUMANTONE_TEST_API_KEY")) == "" {
		// No key — silently no-op so the build tag remains the only gate.
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func newClient(t *testing.T) *humantone.Client {
	t.Helper()
	c, err := humantone.NewClient(humantone.Config{
		APIKey:  os.Getenv("HUMANTONE_TEST_API_KEY"),
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

const sampleText = "Artificial intelligence has fundamentally transformed how content teams approach their daily work. " +
	"Modern writers now routinely use AI tools to draft initial outlines, edit existing prose, and polish the final " +
	"version of articles before they ship to production. The technology continues to improve rapidly each quarter."

func TestIntegration_HumanizeStandard(t *testing.T) {
	c := newClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	res, err := c.Humanize(ctx, humantone.HumanizeRequest{
		Text:  sampleText,
		Level: humantone.LevelStandard,
	})
	if err != nil {
		t.Fatalf("Humanize standard: %v", err)
	}
	if res.Text == "" {
		t.Errorf("empty Text in result")
	}
	if res.CreditsUsed <= 0 {
		t.Errorf("CreditsUsed = %d", res.CreditsUsed)
	}
}

func TestIntegration_HumanizeAdvanced(t *testing.T) {
	c := newClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	res, err := c.Humanize(ctx, humantone.HumanizeRequest{
		Text:  sampleText,
		Level: humantone.LevelAdvanced,
	})
	if err != nil {
		t.Fatalf("Humanize advanced: %v", err)
	}
	if res.Text == "" {
		t.Errorf("empty Text")
	}
}

func TestIntegration_HumanizeExtreme(t *testing.T) {
	c := newClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	res, err := c.Humanize(ctx, humantone.HumanizeRequest{
		Text:  sampleText,
		Level: humantone.LevelExtreme,
	})
	if err != nil {
		t.Fatalf("Humanize extreme: %v", err)
	}
	if res.Text == "" {
		t.Errorf("empty Text")
	}
}

func TestIntegration_Detect(t *testing.T) {
	c := newClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := c.Detect(ctx, sampleText)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if res.AIScore < 0 || res.AIScore > 100 {
		t.Errorf("AIScore = %d, out of range", res.AIScore)
	}
}

func TestIntegration_Account(t *testing.T) {
	c := newClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := c.Account.Get(ctx)
	if err != nil {
		t.Fatalf("Account.Get: %v", err)
	}
	if info.Plan.ID == "" {
		t.Errorf("Plan.ID empty")
	}
	if info.Credits.Total < 0 {
		t.Errorf("Credits.Total negative: %d", info.Credits.Total)
	}
}

func TestIntegration_UnauthorizedKey(t *testing.T) {
	// Structurally valid (matches regex) but never issued by the server.
	c, err := humantone.NewClient(humantone.Config{
		APIKey:  "ht_deadbeef00000000000000000000000000000000000000000000000000000000",
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = c.Account.Get(ctx)
	if !errors.Is(err, humantone.ErrAuthentication) {
		t.Errorf("expected ErrAuthentication, got %v", err)
	}
}
