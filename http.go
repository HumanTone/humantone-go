package humantone

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
)

// doOnce performs a single HTTP roundtrip. It does not consult the retry
// policy — callers (retry.go) decide whether to invoke it again. The body
// argument may be nil for GET requests; for POST it is the marshaled JSON
// payload, which is wrapped in a fresh bytes.Reader on each call so that
// retries do not need to re-marshal.
//
// On a successful HTTP exchange (any status code), doOnce returns the
// response status, headers, fully-read body, and a nil error. On transport
// failure (DNS, connection refused, TLS handshake, context cancellation,
// timeout), it returns the classified *Error and zero-valued status/headers.
func (c *Client) doOnce(ctx context.Context, method, path string, body []byte) (int, http.Header, []byte, *Error) {
	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		// Bad URL or method — surface as ErrInvalidRequest, not retryable.
		e := newError(ErrInvalidRequest, ErrCodeInvalidRequest, err.Error())
		return 0, nil, nil, e
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, classifyTransportError(err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		// Read errors mid-stream are network-shaped; classify as such.
		return 0, nil, nil, classifyTransportError(readErr)
	}

	return resp.StatusCode, resp.Header, respBody, nil
}

// classifyTransportError maps a transport-layer error from net/http into a
// *Error per §7.6.1. context.DeadlineExceeded and timeout-flagged net.Error
// values become ErrTimeout; context.Canceled also becomes ErrTimeout (caller
// cancellation is treated as a timeout-shaped failure rather than a network
// fault). Everything else becomes ErrNetwork.
func classifyTransportError(err error) *Error {
	if errors.Is(err, context.DeadlineExceeded) {
		e := newError(ErrTimeout, ErrCodeTimeout, err.Error())
		return e
	}
	if errors.Is(err, context.Canceled) {
		e := newError(ErrTimeout, ErrCodeTimeout, err.Error())
		return e
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		e := newError(ErrTimeout, ErrCodeTimeout, err.Error())
		return e
	}
	e := newError(ErrNetwork, ErrCodeNetwork, err.Error())
	e.Retryable = true
	return e
}
