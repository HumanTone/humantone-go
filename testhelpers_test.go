package humantone

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// validTestAPIKey is a structurally-valid (matches the regex) HumanTone API
// key the live API has never issued. Safe to use in tests because no real
// HTTP request reaches a real server in unit tests.
const validTestAPIKey = "ht_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// loadFixture reads a JSON fixture from testdata/ and returns its bytes.
// Fails the test on any error — fixtures are part of the source tree and
// missing ones indicate a setup mistake, not a runtime condition.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadFixture(%q): %v", name, err)
	}
	return data
}

// newTestClient builds a *Client wired up to the given base URL with the
// SDK's normal NewClient validation. Use this from tests to ensure the same
// code path as production.
func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	c, err := NewClient(Config{
		APIKey:  validTestAPIKey,
		BaseURL: baseURL,
		// Make retry-failing tests fast — most tests need 0 or 1 retries.
		MaxRetries: 2,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// recordingServer wraps httptest.NewServer with a captured request log and
// per-call response control. It records every incoming request (method,
// path, headers, body) and lets tests script the sequence of responses.
type recordingServer struct {
	server      *httptest.Server
	requests    []recordedRequest
	responses   []scriptedResponse
	callCounter int32
	t           *testing.T
}

type recordedRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

type scriptedResponse struct {
	Status  int
	Headers map[string]string
	Body    []byte
}

// newRecordingServer starts an httptest server backed by the given response
// script. The Nth incoming request gets responses[N]. If more requests
// arrive than responses are scripted, the test fails. Always call
// (*recordingServer).Close() via t.Cleanup to release resources.
func newRecordingServer(t *testing.T, responses []scriptedResponse) *recordingServer {
	t.Helper()
	rs := &recordingServer{t: t, responses: responses}
	rs.server = httptest.NewServer(http.HandlerFunc(rs.handle))
	t.Cleanup(rs.server.Close)
	return rs
}

func (rs *recordingServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := readAll(r)
	rs.requests = append(rs.requests, recordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Header: r.Header.Clone(),
		Body:   body,
	})
	idx := int(atomic.AddInt32(&rs.callCounter, 1)) - 1
	if idx >= len(rs.responses) {
		rs.t.Fatalf("recordingServer: unexpected request #%d (only %d responses scripted)", idx+1, len(rs.responses))
		return
	}
	resp := rs.responses[idx]
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	if resp.Status == 0 {
		resp.Status = http.StatusOK
	}
	w.WriteHeader(resp.Status)
	if len(resp.Body) > 0 {
		_, _ = w.Write(resp.Body)
	}
}

func (rs *recordingServer) URL() string { return rs.server.URL }

// readAll reads an *http.Request body fully and returns its bytes.
func readAll(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// withSilencedSleep replaces sleepFunc with a no-op (still honoring context
// cancellation) for the duration of the test, so retry-loop tests do not
// actually pause. Restored on cleanup.
func withSilencedSleep(t *testing.T) {
	t.Helper()
	orig := sleepFunc
	sleepFunc = func(ctx context.Context, _ time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	}
	t.Cleanup(func() { sleepFunc = orig })
}

// withDeterministicJitter pins jitterFunc to zero for the test duration so
// backoff timing assertions are stable.
func withDeterministicJitter(t *testing.T) {
	t.Helper()
	orig := jitterFunc
	jitterFunc = func() time.Duration { return 0 }
	t.Cleanup(func() { jitterFunc = orig })
}
