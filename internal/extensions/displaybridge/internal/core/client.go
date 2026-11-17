// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Default client behavior. Both are exposed for tests (which override via
// newHTTPClient options) but should not be tuned per call site — uniform
// transport behavior across slots is intentional.
const (
	defaultTimeout = 30 * time.Second
	defaultRetries = 3
	// Initial backoff; doubles on each subsequent attempt: 100ms, 200ms, 400ms.
	// Total worst-case added latency for 3 retries ≈ 700ms — fast enough to
	// not user-visibly stall, slow enough to let a transient 5xx clear.
	initialBackoff = 100 * time.Millisecond
)

// httpClient is the displaybridge transport to the bun sidecar. Single
// instance lives inside the schemaBridge struct (Subtask 3 wires it). All
// public methods reach bun's authenticated /api/* surface; the shared
// retry/error/auth machinery is in do().
//
// Concurrency: safe for use from multiple goroutines (net/http.Client is
// concurrency-safe; EndpointProvider implementations document their own
// safety).
type httpClient struct {
	endpoint EndpointProvider
	hc       *http.Client
	maxRetry int
	backoff  time.Duration
}

// httpClientOpt mutates an httpClient at construction time. Tests use these
// to inject shorter timeouts and a stub *http.Client.
type httpClientOpt func(*httpClient)

func withMaxRetry(n int) httpClientOpt          { return func(c *httpClient) { c.maxRetry = n } }
func withBackoff(d time.Duration) httpClientOpt { return func(c *httpClient) { c.backoff = d } }
func withHTTPClient(h *http.Client) httpClientOpt {
	return func(c *httpClient) {
		if h != nil {
			c.hc = h
		}
	}
}

// newHTTPClient builds an httpClient bound to the given EndpointProvider.
// Uses sane defaults for production; tests pass opts to override.
func newHTTPClient(endpoint EndpointProvider, opts ...httpClientOpt) *httpClient {
	c := &httpClient{
		endpoint: endpoint,
		hc:       &http.Client{Timeout: defaultTimeout},
		maxRetry: defaultRetries,
		backoff:  initialBackoff,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// callOpts is the per-call retry policy.
type callOpts struct {
	retry bool
}

// CallOption is a per-call modifier.
type CallOption func(*callOpts)

// WithRetry enables retry-on-5xx + transient network errors for this call.
// Use only for idempotent operations — repeating a non-idempotent call after
// a 5xx may produce duplicate side effects on the bun side.
func WithRetry() CallOption { return func(o *callOpts) { o.retry = true } }

// Get performs an authenticated GET against bun. GET is idempotent by HTTP
// semantics, so retry is enabled by default.
//
// path is the URL path beginning with "/" (e.g. "/api/types"). out, if
// non-nil, must be a pointer; the JSON response body is decoded into it.
func (c *httpClient) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out, callOpts{retry: true})
}

// Post performs an authenticated POST with a JSON-encoded body. Retry is
// disabled by default — caller must opt-in via WithRetry for slots that are
// idempotent (Parse, Validate, Generate*, GetWidgetURL, ...).
//
// in, if non-nil, is JSON-marshalled into the request body. out, if
// non-nil, must be a pointer for response decoding.
func (c *httpClient) Post(ctx context.Context, path string, in, out any, opts ...CallOption) error {
	o := callOpts{retry: false}
	for _, opt := range opts {
		opt(&o)
	}
	return c.do(ctx, http.MethodPost, path, in, out, o)
}

// PostJSON is a convenience wrapper for idempotent POST: identical to
// Post(..., WithRetry()). Used by read-only POST slots (parse, validate,
// generate, etc.) where a 5xx retry is safe.
func (c *httpClient) PostJSON(ctx context.Context, path string, in, out any) error {
	return c.do(ctx, http.MethodPost, path, in, out, callOpts{retry: true})
}

// GetRaw performs an authenticated GET and returns the raw response body
// without JSON decoding. Used for slots whose response payload is not JSON
// (e.g. GetTSXFile returning TypeScript source bytes).
func (c *httpClient) GetRaw(ctx context.Context, path string) ([]byte, error) {
	base := c.endpoint.BaseURL()
	if base == "" {
		return nil, ErrEndpointUnavailable
	}
	url := strings.TrimRight(base, "/") + path

	attempts := c.maxRetry + 1
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			d := c.backoff * (1 << uint(i-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(d):
			}
		}
		body, retryable, err := c.attemptRaw(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

// attemptRaw is the GetRaw equivalent of attempt — returns the body bytes
// directly on 2xx; on non-2xx, maps to BridgeError without trying to
// JSON-decode the response.
func (c *httpClient) attemptRaw(ctx context.Context, url string) (body []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("displaybridge: build request: %w", err)
	}
	if tok := c.endpoint.Token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, err
		}
		return nil, true, fmt.Errorf("displaybridge: transport: %w", err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, true, fmt.Errorf("displaybridge: read response: %w", readErr)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return respBody, false, nil
	}
	be := mapErrorResponse(resp.StatusCode, respBody)
	return nil, be.IsTransient(), be
}

// do is the single request execution path: build URL, marshal body, attach
// auth, send, decode, retry on 5xx + network errors when allowed.
func (c *httpClient) do(ctx context.Context, method, path string, in, out any, opts callOpts) error {
	base := c.endpoint.BaseURL()
	if base == "" {
		return ErrEndpointUnavailable
	}
	url := strings.TrimRight(base, "/") + path

	var bodyBytes []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("displaybridge: marshal request: %w", err)
		}
		bodyBytes = b
	}

	attempts := 1
	if opts.retry {
		attempts = c.maxRetry + 1
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			// Exponential backoff: 100ms, 200ms, 400ms ...
			d := c.backoff * (1 << uint(i-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}

		retryable, err := c.attempt(ctx, method, url, bodyBytes, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable {
			return err
		}
		// retryable: loop continues if attempts remain
	}
	return lastErr
}

// attempt sends one HTTP request and decodes the response. The boolean
// return indicates whether the error is safe to retry (5xx, transient
// network errors). Non-retryable errors (4xx, marshal failures, ctx
// cancellations) return retryable=false.
func (c *httpClient) attempt(ctx context.Context, method, url string, body []byte, out any) (retryable bool, err error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return false, fmt.Errorf("displaybridge: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if tok := c.endpoint.Token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		// Context cancellation/deadline is non-retryable; transport errors
		// (connection refused, timeout) are retryable.
		if ctx.Err() != nil {
			return false, err
		}
		return true, fmt.Errorf("displaybridge: transport: %w", err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		// Mid-stream read failure — treat as transient.
		return true, fmt.Errorf("displaybridge: read response: %w", readErr)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return false, fmt.Errorf("displaybridge: decode response: %w", err)
			}
		}
		return false, nil
	}

	be := mapErrorResponse(resp.StatusCode, respBody)
	// Retry only on transient 5xx. 4xx is the caller's fault (bad request,
	// auth, not-found) and 501 is a permanent Phase-1 state — repeating
	// won't help.
	return be.IsTransient(), be
}
