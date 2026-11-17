// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SSE backoff bounds. The bridge tolerates bun being down at startup or
// crashing mid-session; the run loop reconnects with exponential backoff.
//
// Clamps are wide enough to recover from transient failures quickly (1s)
// while not hammering bun during permanent outages (30s ceiling).
const (
	sseBackoffMin = 1 * time.Second
	sseBackoffMax = 30 * time.Second
	// A stream that survived this long counts as "healthy" — its drop
	// resets the backoff to the minimum so the next reconnect is fast.
	sseHealthyDuration = 30 * time.Second
)

// sseEvent is a single parsed Server-Sent Event. Data is the raw event
// payload as a string (typically JSON); the handler decodes it.
type sseEvent struct {
	Type string
	Data string
}

// sseSubscriber maintains a long-lived connection to bun's
// /sse/schema-changed stream and routes incoming events to cache
// invalidations. Best-effort: if the stream can't be established (bun
// down, network partition), the cache simply doesn't get invalidated by
// remote events — the validator still works, it just falls back to
// fetch-on-miss for every call until SSE reconnects.
//
// Lifecycle:
//
//	newSSESubscriber → Start (idempotent, fires the run loop in a goroutine)
//	                  → Stop  (cancels + waits for the goroutine to exit)
//
// The subscriber owns its own context, decoupled from any per-call ctx.
// Stop is safe to call multiple times and from any goroutine.
type sseSubscriber struct {
	endpoint   EndpointProvider
	cache      *schemaCache
	httpClient *http.Client

	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	once    sync.Once
	started atomic.Bool

	backoffMin time.Duration
	backoffMax time.Duration
}

type sseSubscriberOpt func(*sseSubscriber)

func sseWithBackoff(min, max time.Duration) sseSubscriberOpt {
	return func(s *sseSubscriber) {
		if min > 0 {
			s.backoffMin = min
		}
		if max > 0 {
			s.backoffMax = max
		}
	}
}

// sseWithHTTPClient overrides the streaming HTTP client (tests use this to
// inject httptest.Server's Client, which allows redirect-following config).
func sseWithHTTPClient(hc *http.Client) sseSubscriberOpt {
	return func(s *sseSubscriber) {
		if hc != nil {
			s.httpClient = hc
		}
	}
}

// newSSESubscriber builds a subscriber bound to endpoint + cache. Use
// sseWithBackoff to tighten timing in tests.
//
// Note the streaming HTTP client has NO Timeout — SSE connections are
// long-lived. Cancellation via the subscriber's internal context is the
// only way the connection ends gracefully from our side.
func newSSESubscriber(endpoint EndpointProvider, cache *schemaCache, opts ...sseSubscriberOpt) *sseSubscriber {
	ctx, cancel := context.WithCancel(context.Background())
	s := &sseSubscriber{
		endpoint:   endpoint,
		cache:      cache,
		httpClient: &http.Client{}, // no Timeout!
		ctx:        ctx,
		cancel:     cancel,
		done:       make(chan struct{}),
		backoffMin: sseBackoffMin,
		backoffMax: sseBackoffMax,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start launches the connect-stream-reconnect loop in a goroutine.
// Idempotent: subsequent calls are no-ops.
func (s *sseSubscriber) Start() {
	s.once.Do(func() {
		s.started.Store(true)
		go s.run()
	})
}

// Stop cancels the subscriber's context and (if Start was called) waits
// up to 2s for the run loop to exit. The 2s ceiling protects against
// goroutines blocked in Body.Read on streams whose write side is
// unresponsive — in that case we leak the goroutine rather than block
// the caller.
//
// Safe to call multiple times. Safe to call before Start: Stop without
// a prior Start is a fast no-op (no goroutine to wait for).
func (s *sseSubscriber) Stop() {
	s.cancel()
	if !s.started.Load() {
		return
	}
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		// goroutine stuck on Body.Read; leak rather than block.
	}
}

// run is the reconnect loop. Each iteration: connect, stream until
// disconnect, clear cache (safer than serving stale), backoff, repeat.
func (s *sseSubscriber) run() {
	defer close(s.done)

	backoff := s.backoffMin
	for {
		if s.ctx.Err() != nil {
			return
		}

		startedAt := time.Now()
		_ = s.connectAndStream(s.ctx)

		// If the stream lasted long enough to count as healthy, reset
		// backoff so the next reconnect doesn't carry punishment from
		// previous failures.
		if time.Since(startedAt) > sseHealthyDuration {
			backoff = s.backoffMin
		}

		// Cache invalidation policy on disconnect: drop everything. We
		// can't tell what changed during the gap, so re-fetching on
		// next-use is safer than potentially serving stale schemas.
		s.cache.Clear()

		// Backoff before reconnecting.
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = nextBackoff(backoff, s.backoffMax)
	}
}

// connectAndStream opens GET /sse/schema-changed and feeds the response
// body into the SSE parser. Returns when the stream ends (EOF), errors,
// or the context is cancelled.
func (s *sseSubscriber) connectAndStream(ctx context.Context) error {
	base := s.endpoint.BaseURL()
	if base == "" {
		return ErrEndpointUnavailable
	}
	url := strings.TrimRight(base, "/") + "/sse/schema-changed"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if tok := s.endpoint.Token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("displaybridge sse: unexpected status %d", resp.StatusCode)
	}

	return parseSSEStream(resp.Body, s.handleEvent)
}

// handleEvent dispatches a parsed event to the cache. Only
// "schema-changed" events trigger invalidation; everything else (including
// the unnamed default and heartbeats) is ignored.
//
// Payload shape (matches bun's emit):
//
//	{"name": "<schemaName>", "version": <int>}
func (s *sseSubscriber) handleEvent(ev sseEvent) {
	if ev.Type != "schema-changed" {
		return
	}
	var payload struct {
		Name    string `json:"name"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(ev.Data), &payload); err != nil {
		return
	}
	if payload.Name == "" {
		return
	}
	s.cache.Invalidate(payload.Name)
}

// parseSSEStream reads a Server-Sent Events stream from r and invokes
// onEvent for each complete event. Implements the parsing rules from the
// HTML5 SSE spec subset we need:
//
//   - Lines are CR/LF/CRLF terminated. bufio.Scanner.Split=ScanLines
//     handles all three.
//   - A blank line ends an event.
//   - Lines starting with ":" are comments — ignored. (Also used as
//     keep-alive heartbeats.)
//   - "field: value" — value's leading single space is stripped per spec.
//   - "data:" lines accumulate; multiple data lines join with "\n".
//   - Recognized fields: event, data. id and retry are accepted but
//     unused in Phase 1 (no last-event-id resume yet).
//
// Returns when the reader hits EOF or a scanner error. Caller-side
// context cancellation closes resp.Body, which surfaces here as a read
// error from the underlying Scanner.
func parseSSEStream(r io.Reader, onEvent func(ev sseEvent)) error {
	scanner := bufio.NewScanner(r)
	// Allow generous data lines (JSON payloads can be large).
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var ev sseEvent
	hasField := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if hasField {
				onEvent(ev)
			}
			ev = sseEvent{}
			hasField = false
			continue
		}
		if strings.HasPrefix(line, ":") {
			// Comment / heartbeat — ignore.
			continue
		}
		colon := strings.IndexByte(line, ':')
		var field, value string
		if colon < 0 {
			// Line without ':' — per spec, treat the whole line as the
			// field with empty value.
			field = line
		} else {
			field = line[:colon]
			value = line[colon+1:]
			if strings.HasPrefix(value, " ") {
				value = value[1:]
			}
		}
		switch field {
		case "event":
			ev.Type = value
			hasField = true
		case "data":
			if ev.Data == "" {
				ev.Data = value
			} else {
				ev.Data += "\n" + value
			}
			hasField = true
		case "id", "retry":
			// Accepted but ignored in Phase 1.
		}
	}
	return scanner.Err()
}

// nextBackoff doubles d, capped at max. Pure function for unit testing.
func nextBackoff(d, max time.Duration) time.Duration {
	d *= 2
	if d > max {
		return max
	}
	return d
}
