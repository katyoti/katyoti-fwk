// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"sync"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// A CALL SENT AGAIN LANDS ONCE.
//
// A caller whose answer was lost - the reply dropped, a « Slot not found » or
// a timeout after the write had landed - sends the call again. Every call
// carries a request id (HeaderRequestID), minted once per logical call and
// kept across all its retries (RetrySender puts it in the context; Sender
// reads it there, or mints one). The serving side keeps, per request id, the
// answer it gave in a bounded replay cache: an id it already answered is
// answered again from the cache without running the method; an id whose first
// run is still in flight waits for that run's answer. A call with no request
// id (an older caller) runs as before.
//
// The cache is bounded in BYTES as well as in time and count:
// it once kept every answer ten minutes whatever its size, and the hub, which
// answers multi-MiB manifest slot types and index pages to every command, held
// gigabytes of answers no retry would ever ask again. An answer larger than
// replayMaxEntryBytes is given to the callers waiting on its run, then not
// kept: a retry of it runs again (such answers are reads - a write's answer is
// small); the answers kept total at most replayMaxBytes, the oldest going
// first.

// HeaderRequestID carries the id of one logical call, the same on each of its
// retries - the key of the serving side's replay cache. A retry is a new
// transport call (a new HeaderCallID) of the same request.
const HeaderRequestID = "ktw.meta.requestId"

type requestIDKey struct{}

// WithRequestID is ctx carrying the request id every call made with it sends:
// the retries of one logical call share it.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFrom is the request id ctx carries, "" when none.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// RequestIDFromMessage is the request id an inbound request carries, "" when
// none.
func RequestIDFromMessage(msg mbapi.Message) string {
	if h := msg.Header(); h != nil {
		if v := h[HeaderRequestID]; len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// The replay cache's bounds: an answer is kept this long, at most this many
// answers and this many bytes of them are kept (the oldest finished ones go
// first), and an answer larger than replayMaxEntryBytes is not kept at all.
const (
	replayTTL           = 10 * time.Minute
	replayMax           = 4096
	replayMaxBytes      = 32 << 20
	replayMaxEntryBytes = 256 << 10
)

// replayCache is the answers a serving subscription gave, by request id.
type replayCache struct {
	mu       sync.Mutex
	entries  map[string]*replayEntry
	ttl      time.Duration
	max      int
	maxBytes int // the answers kept total at most this
	maxEntry int // an answer larger than this is not kept
	bytes    int // the answers kept, in bytes
	now      func() time.Time
}

// replayEntry is one request's answer: done closes once body holds it.
type replayEntry struct {
	key  string
	done chan struct{}
	body []byte
	at   time.Time // when it was answered (zero while in flight)
}

func newReplayCache() *replayCache {
	return &replayCache{entries: map[string]*replayEntry{}, ttl: replayTTL, max: replayMax,
		maxBytes: replayMaxBytes, maxEntry: replayMaxEntryBytes, now: time.Now}
}

// begin answers the entry of key and whether this caller runs it: the first
// caller of an id (or of an expired one) owns it and must finish it; a later
// one waits on its done.
func (c *replayCache) begin(key string) (*replayEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if e, ok := c.entries[key]; ok {
		if e.at.IsZero() || now.Sub(e.at) < c.ttl {
			return e, false
		}
		c.drop(key, e) // expired: it runs again
	}
	c.prune(now)
	e := &replayEntry{key: key, done: make(chan struct{})}
	c.entries[key] = e
	return e, true
}

// finish records the answer the owner of e gave, and wakes who waits on it.
// The callers already waiting read it from e; the cache keeps it only when it
// is small enough, then evicts the oldest answers past its byte budget.
func (c *replayCache) finish(e *replayEntry, body []byte) {
	c.mu.Lock()
	e.body, e.at = body, c.now()
	if c.entries[e.key] == e { // still the cache's (not expired and run again)
		if len(body) > c.maxEntry {
			delete(c.entries, e.key) // too large to keep: a retry runs again
		} else {
			c.bytes += len(body)
			c.prune(e.at)
		}
	}
	c.mu.Unlock()
	close(e.done)
}

// drop removes key's entry, counting its bytes out. Called with mu held.
func (c *replayCache) drop(key string, e *replayEntry) {
	delete(c.entries, key)
	c.bytes -= len(e.body)
}

// prune drops the expired answers, then the oldest finished ones while more
// than max answers or maxBytes bytes are kept. Called with mu held.
func (c *replayCache) prune(now time.Time) {
	var oldestKey string
	var oldest time.Time
	for k, e := range c.entries {
		if e.at.IsZero() {
			continue
		}
		if now.Sub(e.at) >= c.ttl {
			c.drop(k, e)
			continue
		}
		if oldestKey == "" || e.at.Before(oldest) {
			oldestKey, oldest = k, e.at
		}
	}
	for (len(c.entries) >= c.max || c.bytes > c.maxBytes) && oldestKey != "" {
		c.drop(oldestKey, c.entries[oldestKey])
		oldestKey = ""
		for k, e := range c.entries {
			if !e.at.IsZero() && (oldestKey == "" || e.at.Before(oldest)) {
				oldestKey, oldest = k, e.at
			}
		}
	}
}
