// SPDX-License-Identifier: Apache-2.0

package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- parseSSEStream tests ---

func TestParseSSE_SingleEvent(t *testing.T) {
	body := "event: schema-changed\ndata: {\"name\":\"User\"}\n\n"
	var got []sseEvent
	if err := parseSSEStream(strings.NewReader(body), func(ev sseEvent) {
		got = append(got, ev)
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if got[0].Type != "schema-changed" {
		t.Errorf("Type = %q, want schema-changed", got[0].Type)
	}
	if got[0].Data != `{"name":"User"}` {
		t.Errorf("Data = %q, want JSON payload", got[0].Data)
	}
}

func TestParseSSE_MultipleEvents(t *testing.T) {
	body := "event: schema-changed\ndata: {\"name\":\"A\"}\n\nevent: schema-changed\ndata: {\"name\":\"B\"}\n\n"
	var got []sseEvent
	_ = parseSSEStream(strings.NewReader(body), func(ev sseEvent) { got = append(got, ev) })
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if got[0].Data != `{"name":"A"}` || got[1].Data != `{"name":"B"}` {
		t.Errorf("event payloads wrong: %+v", got)
	}
}

func TestParseSSE_HeartbeatComment_Ignored(t *testing.T) {
	body := ": heartbeat\n\n: heartbeat\n\nevent: schema-changed\ndata: {\"name\":\"U\"}\n\n"
	var got []sseEvent
	_ = parseSSEStream(strings.NewReader(body), func(ev sseEvent) { got = append(got, ev) })
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1 (heartbeats must be ignored)", len(got))
	}
}

func TestParseSSE_MultiLineData(t *testing.T) {
	body := "event: notice\ndata: line1\ndata: line2\ndata: line3\n\n"
	var got []sseEvent
	_ = parseSSEStream(strings.NewReader(body), func(ev sseEvent) { got = append(got, ev) })
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if got[0].Data != "line1\nline2\nline3" {
		t.Errorf("Data = %q, want joined-with-newline", got[0].Data)
	}
}

func TestParseSSE_LeadingSpaceStripped(t *testing.T) {
	// SSE spec: a single leading space after the colon is stripped.
	body := "event: x\ndata: payload\n\n"
	var got []sseEvent
	_ = parseSSEStream(strings.NewReader(body), func(ev sseEvent) { got = append(got, ev) })
	if got[0].Data != "payload" {
		t.Errorf("Data = %q, want 'payload' (leading space stripped)", got[0].Data)
	}
	if got[0].Type != "x" {
		t.Errorf("Type = %q, want 'x'", got[0].Type)
	}
}

func TestParseSSE_IDAndRetry_Ignored(t *testing.T) {
	body := "id: 123\nretry: 5000\nevent: schema-changed\ndata: {}\n\n"
	var got []sseEvent
	_ = parseSSEStream(strings.NewReader(body), func(ev sseEvent) { got = append(got, ev) })
	if len(got) != 1 || got[0].Type != "schema-changed" {
		t.Errorf("want 1 schema-changed event, got %+v", got)
	}
}

func TestParseSSE_BlankInputProducesNoEvent(t *testing.T) {
	count := 0
	_ = parseSSEStream(strings.NewReader(""), func(_ sseEvent) { count++ })
	if count != 0 {
		t.Errorf("got %d events, want 0", count)
	}
}

func TestParseSSE_TrailingDataNoBlankLine_NotEmitted(t *testing.T) {
	// Per spec, an event is emitted only on a blank line. Without one,
	// the partial event is held until the next read.
	body := "event: x\ndata: payload\n"
	var got []sseEvent
	_ = parseSSEStream(strings.NewReader(body), func(ev sseEvent) { got = append(got, ev) })
	if len(got) != 0 {
		t.Errorf("got %d events, want 0 (no terminator)", len(got))
	}
}

func TestNextBackoff_DoublingAndCap(t *testing.T) {
	cases := []struct {
		in, max, want time.Duration
	}{
		{1 * time.Second, 30 * time.Second, 2 * time.Second},
		{2 * time.Second, 30 * time.Second, 4 * time.Second},
		{16 * time.Second, 30 * time.Second, 30 * time.Second}, // 32 → capped
		{30 * time.Second, 30 * time.Second, 30 * time.Second},
	}
	for _, c := range cases {
		if got := nextBackoff(c.in, c.max); got != c.want {
			t.Errorf("nextBackoff(%v, %v) = %v, want %v", c.in, c.max, got, c.want)
		}
	}
}

// --- sseSubscriber: end-to-end with httptest server ---

// sseTestServer is a controllable bun stub: callers can push events
// through the events channel; closing the connection-control channel
// drops the active connection.
type sseTestServer struct {
	t            *testing.T
	srv          *httptest.Server
	connections  atomic.Int32
	events       chan string // strings written verbatim to the stream
	dropAfter    time.Duration
	notSubscribe atomic.Bool // when true, return 500 immediately
}

func newSSETestServer(t *testing.T) *sseTestServer {
	t.Helper()
	s := &sseTestServer{
		t:      t,
		events: make(chan string, 16),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/sse/schema-changed", s.handle)
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *sseTestServer) handle(w http.ResponseWriter, r *http.Request) {
	if s.notSubscribe.Load() {
		http.Error(w, "down", http.StatusInternalServerError)
		return
	}
	s.connections.Add(1)
	defer s.connections.Add(-1)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flush", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()

	var dropTimer <-chan time.Time
	if s.dropAfter > 0 {
		dropTimer = time.After(s.dropAfter)
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-s.events:
			if !open {
				return
			}
			_, _ = w.Write([]byte(ev))
			flusher.Flush()
		case <-dropTimer:
			return
		}
	}
}

func (s *sseTestServer) push(record string) {
	s.events <- record
}

func (s *sseTestServer) URL() string {
	return s.srv.URL
}

// waitForCondition polls f until it returns true or the timeout elapses.
// Test helper; avoids hand-rolled select/sleep loops in each test.
func waitForCondition(t *testing.T, timeout time.Duration, f func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func TestSSE_InvalidatesCacheOnSchemaChanged(t *testing.T) {
	srv := newSSETestServer(t)
	cache := newSchemaCache(8)
	cache.Put("Book", 1, stubSchema(t, "Book"))
	cache.Put("Other", 1, stubSchema(t, "Other"))

	sub := newSSESubscriber(
		NewStaticEndpoint(srv.URL(), ""),
		cache,
		sseWithBackoff(10*time.Millisecond, 100*time.Millisecond),
	)
	sub.Start()
	defer sub.Stop()

	if !waitForCondition(t, time.Second, func() bool { return srv.connections.Load() >= 1 }) {
		t.Fatal("subscriber never connected")
	}

	srv.push(`event: schema-changed` + "\n" + `data: {"name":"Book","version":2}` + "\n\n")

	if !waitForCondition(t, time.Second, func() bool {
		_, _, ok := cache.Get("Book")
		return !ok
	}) {
		t.Fatal("Book never invalidated")
	}
	// Other schemas must remain.
	if _, _, ok := cache.Get("Other"); !ok {
		t.Error("Other was invalidated; only Book should have been")
	}
}

func TestSSE_IgnoresUnknownEventTypes(t *testing.T) {
	srv := newSSETestServer(t)
	cache := newSchemaCache(8)
	cache.Put("X", 1, stubSchema(t, "X"))
	sub := newSSESubscriber(
		NewStaticEndpoint(srv.URL(), ""),
		cache,
		sseWithBackoff(10*time.Millisecond, 100*time.Millisecond),
	)
	sub.Start()
	defer sub.Stop()

	if !waitForCondition(t, time.Second, func() bool { return srv.connections.Load() >= 1 }) {
		t.Fatal("not connected")
	}

	// Unknown event types are no-ops on cache.
	srv.push(`event: weather` + "\n" + `data: {"name":"X"}` + "\n\n")
	srv.push(`: heartbeat` + "\n\n") // comment / heartbeat

	// Sleep briefly so any (incorrect) handler could have fired.
	time.Sleep(50 * time.Millisecond)
	if _, _, ok := cache.Get("X"); !ok {
		t.Error("X invalidated by unknown event type")
	}
}

func TestSSE_IgnoresMalformedJSON(t *testing.T) {
	srv := newSSETestServer(t)
	cache := newSchemaCache(8)
	cache.Put("X", 1, stubSchema(t, "X"))
	sub := newSSESubscriber(
		NewStaticEndpoint(srv.URL(), ""),
		cache,
		sseWithBackoff(10*time.Millisecond, 100*time.Millisecond),
	)
	sub.Start()
	defer sub.Stop()

	if !waitForCondition(t, time.Second, func() bool { return srv.connections.Load() >= 1 }) {
		t.Fatal("not connected")
	}

	srv.push(`event: schema-changed` + "\n" + `data: {invalid json` + "\n\n")
	srv.push(`event: schema-changed` + "\n" + `data: {"name":""}` + "\n\n") // empty name

	time.Sleep(50 * time.Millisecond)
	if _, _, ok := cache.Get("X"); !ok {
		t.Error("X invalidated despite malformed/empty payloads")
	}
}

func TestSSE_DisconnectClearsCacheAndReconnects(t *testing.T) {
	srv := newSSETestServer(t)
	srv.dropAfter = 50 * time.Millisecond

	cache := newSchemaCache(8)
	cache.Put("A", 1, stubSchema(t, "A"))
	cache.Put("B", 1, stubSchema(t, "B"))

	sub := newSSESubscriber(
		NewStaticEndpoint(srv.URL(), ""),
		cache,
		sseWithBackoff(10*time.Millisecond, 50*time.Millisecond),
	)
	sub.Start()
	defer sub.Stop()

	// Wait for first connect.
	if !waitForCondition(t, time.Second, func() bool { return srv.connections.Load() >= 1 }) {
		t.Fatal("first connect timed out")
	}

	// After dropAfter elapses, the connection is dropped server-side, the
	// run loop exits the parser, clears the cache, and reconnects.
	if !waitForCondition(t, time.Second, func() bool { return cache.Size() == 0 }) {
		t.Fatal("cache never cleared after disconnect")
	}

	// Subscriber should reconnect.
	if !waitForCondition(t, 2*time.Second, func() bool { return srv.connections.Load() >= 1 }) {
		t.Fatal("did not reconnect after disconnect")
	}
}

func TestSSE_StopHaltsLoop(t *testing.T) {
	srv := newSSETestServer(t)
	cache := newSchemaCache(8)
	sub := newSSESubscriber(
		NewStaticEndpoint(srv.URL(), ""),
		cache,
		sseWithBackoff(10*time.Millisecond, 50*time.Millisecond),
	)
	sub.Start()

	// Wait for connection so we know the goroutine is in the streaming
	// path (not just the backoff sleep).
	if !waitForCondition(t, time.Second, func() bool { return srv.connections.Load() >= 1 }) {
		t.Fatal("not connected")
	}

	stopped := make(chan struct{})
	go func() { sub.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not return within 3s")
	}

	// After Stop, the connection must drop.
	if !waitForCondition(t, time.Second, func() bool { return srv.connections.Load() == 0 }) {
		t.Error("connection still open after Stop")
	}
}

func TestSSE_StartIsIdempotent(t *testing.T) {
	srv := newSSETestServer(t)
	cache := newSchemaCache(8)
	sub := newSSESubscriber(
		NewStaticEndpoint(srv.URL(), ""),
		cache,
		sseWithBackoff(10*time.Millisecond, 50*time.Millisecond),
	)
	defer sub.Stop()

	// Start many times concurrently — should produce only ONE goroutine.
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); sub.Start() }()
	}
	wg.Wait()

	// Wait for connection.
	if !waitForCondition(t, time.Second, func() bool { return srv.connections.Load() >= 1 }) {
		t.Fatal("not connected")
	}

	// Only one goroutine implies only one connection exists at a time
	// (unrelated reconnects after backoff don't happen here because the
	// stream is healthy).
	time.Sleep(100 * time.Millisecond)
	if got := srv.connections.Load(); got != 1 {
		t.Errorf("connections = %d, want 1 (Start must be idempotent)", got)
	}
}

func TestSSE_StopBeforeStart_NoOp(t *testing.T) {
	srv := newSSETestServer(t)
	cache := newSchemaCache(8)
	sub := newSSESubscriber(
		NewStaticEndpoint(srv.URL(), ""),
		cache,
		sseWithBackoff(10*time.Millisecond, 50*time.Millisecond),
	)
	// Should not panic, should not block.
	done := make(chan struct{})
	go func() { sub.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Stop without Start blocked")
	}
}

func TestSSE_NoEndpoint_LoopWaitsAndDoesNotPanic(t *testing.T) {
	cache := newSchemaCache(8)
	sub := newSSESubscriber(
		NewStaticEndpoint("", ""),
		cache,
		sseWithBackoff(10*time.Millisecond, 50*time.Millisecond),
	)
	sub.Start()
	defer sub.Stop()

	// Loop should be alive (not panicking) — give it some iterations.
	time.Sleep(150 * time.Millisecond)
	// Sanity: nothing crashed; cache stays empty.
	if cache.Size() != 0 {
		t.Errorf("cache size = %d after no-endpoint loop, want 0", cache.Size())
	}
}

// --- Bridge integration: New starts SSE, Close stops it ---

func TestSchemaBridge_CloseStopsSSE(t *testing.T) {
	srv := newSSETestServer(t)
	b := newWithEndpoint(NewStaticEndpoint(srv.URL(), ""))
	// Override SSE backoff on the underlying subscriber for fast test.
	b.sse = newSSESubscriber(
		NewStaticEndpoint(srv.URL(), ""),
		b.cache,
		sseWithBackoff(10*time.Millisecond, 50*time.Millisecond),
	)
	b.sse.Start()

	if !waitForCondition(t, time.Second, func() bool { return srv.connections.Load() >= 1 }) {
		t.Fatal("not connected")
	}

	if err := b.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if !waitForCondition(t, time.Second, func() bool { return srv.connections.Load() == 0 }) {
		t.Error("connection still open after bridge Close")
	}
}

// TestSSE_EndToEnd_CacheInvalidationFlow — REMOVED 2026-05-22 (schema-realign).
// Exercised cache invalidation through validateValue → bun's /api/gen/json-schema.
// Both the forwarder + endpoint are gone; the cache no longer has any populator
// on the bridge side. SSE primitives are still covered by the TestParseSSE_* +
// TestSSE_* unit tests above. If the cache + SSE are kept long-term (per the
// slots.go note about observability), a smaller cache-invalidation test should
// be written that pokes the cache directly rather than through a dead method.
