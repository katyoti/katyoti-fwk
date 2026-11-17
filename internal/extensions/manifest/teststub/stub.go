// SPDX-License-Identifier: Apache-2.0

// Package teststub provides an in-memory MessageBus implementation for unit
// tests. No NATS dependency — fast startup, deterministic behaviour, no I/O.
//
// Coverage of the production NATS adapter:
//   - Publish / Subscribe / QueueSubscribe / Request — full
//   - Subject wildcards (* single token, > multi-token) — full
//   - JetStream KV — Put / Get / Delete / Watch / UpdateCAS — full
//   - JetStream Stream Publish + PullConsumer — minimal (in-memory append log)
//   - Drain — full (waits for in-flight handlers to return)
//
// What it does NOT emulate:
//   - TTL on KV (PutWithTTL is a regular Put; no expiry)
//   - Slow-consumer detection
//   - Network failure modes / reconnect
//
// For tests that need TTL or failure simulation, use embedded NATS
// (internal/kts/transport/nats) — but pay the ~200ms startup cost.
package teststub

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// New returns a fresh in-memory MessageBus.
func New() *Stub {
	return &Stub{
		subs:    map[string][]*sub{},
		streams: map[string]*memStream{},
		buckets: map[string]*memKV{},
	}
}

// Stub is the in-memory MessageBus.
//
// Thread-safe: protected by an RWMutex on the subscription map and per-stream
// / per-bucket mutexes.
type Stub struct {
	mu      sync.RWMutex
	subs    map[string][]*sub // exact-subject and wildcard subscriptions; key = pattern string
	streams map[string]*memStream
	buckets map[string]*memKV
	closed  atomic.Bool

	requestTimeout atomic.Int64 // the Request fallback (ns); 0 = the adapter's 5s
}

// SetRequestTimeout sets how long a Request waits when neither its options nor
// its context name a deadline — the transport default, 5s as on NATS. A test
// shortens it to show a caller honours a method's declared timeout without
// waiting seconds.
func (s *Stub) SetRequestTimeout(d time.Duration) { s.requestTimeout.Store(int64(d)) }

type sub struct {
	pattern    string // the subject filter (may contain * and >)
	queueGroup string // empty for non-queue subscribers
	handler    mbapi.Handler
	stub       *Stub
	dead       atomic.Bool
	inFlight   sync.WaitGroup
}

// ----------------------------------------------------------------------------
// MessageBus implementation
// ----------------------------------------------------------------------------

func (s *Stub) Publish(ctx context.Context, subject string, payload []byte, opts ...mbapi.PubOpt) error {
	if s.closed.Load() {
		return errors.New("stub: bus closed")
	}
	po := mbapi.ExtractPubOpts(opts)
	msg := &message{subject: subject, data: payload, header: po.Header}
	s.deliver(msg, "")
	return nil
}

func (s *Stub) Subscribe(ctx context.Context, subject string, handler mbapi.Handler, opts ...mbapi.SubOpt) (mbapi.Subscription, error) {
	if s.closed.Load() {
		return nil, errors.New("stub: bus closed")
	}
	return s.installSub(subject, "", handler), nil
}

func (s *Stub) QueueSubscribe(ctx context.Context, subject, queueGroup string, handler mbapi.Handler, opts ...mbapi.SubOpt) (mbapi.Subscription, error) {
	if s.closed.Load() {
		return nil, errors.New("stub: bus closed")
	}
	if queueGroup == "" {
		return nil, errors.New("stub: queueGroup must be non-empty")
	}
	return s.installSub(subject, queueGroup, handler), nil
}

func (s *Stub) Request(ctx context.Context, subject string, payload []byte, opts ...mbapi.ReqOpt) ([]byte, error) {
	if s.closed.Load() {
		return nil, errors.New("stub: bus closed")
	}
	ro := mbapi.ExtractReqOpts(opts)
	fallback := 5 * time.Second
	if d := time.Duration(s.requestTimeout.Load()); d > 0 {
		fallback = d
	}
	timeout := timeoutFromCtx(ctx, ro.Timeout, fallback)

	// Synthesize a reply subject; exactly one handler will get the request,
	// and we wait for one Respond() call.
	reply := make(chan []byte, 1)
	replyErr := make(chan error, 1)

	msg := &message{
		subject: subject,
		data:    payload,
		header:  ro.Header,
		reply:   "_INBOX." + atomicID(),
		respondFunc: func(payload []byte) error {
			select {
			case reply <- payload:
				return nil
			default:
				return errors.New("stub: respond on dead reply channel")
			}
		},
	}

	delivered := s.deliver(msg, "")
	if delivered == 0 {
		return nil, mbapi.ErrNoResponders{Subject: subject}
	}

	select {
	case data := <-reply:
		return data, nil
	case err := <-replyErr:
		return nil, err
	case <-time.After(timeout):
		return nil, mbapi.ErrTimeout{Subject: subject}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Stub) JetStream() mbapi.JetStreamContext {
	return &jsContext{stub: s}
}

func (s *Stub) Drain(ctx context.Context) error {
	s.closed.Store(true)
	// Wait for in-flight handlers to drain across all subs.
	s.mu.RLock()
	all := []*sub{}
	for _, list := range s.subs {
		all = append(all, list...)
	}
	s.mu.RUnlock()

	done := make(chan struct{})
	go func() {
		for _, sb := range all {
			sb.dead.Store(true)
			sb.inFlight.Wait()
		}
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ----------------------------------------------------------------------------
// Subscription management
// ----------------------------------------------------------------------------

func (s *Stub) installSub(pattern, queueGroup string, handler mbapi.Handler) *sub {
	sb := &sub{pattern: pattern, queueGroup: queueGroup, handler: handler, stub: s}
	s.mu.Lock()
	s.subs[pattern] = append(s.subs[pattern], sb)
	s.mu.Unlock()
	return sb
}

func (s *Stub) removeSub(victim *sub) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.subs[victim.pattern]
	for i, x := range list {
		if x == victim {
			s.subs[victim.pattern] = append(list[:i], list[i+1:]...)
			break
		}
	}
}

// deliver routes the message to all matching subs. Queue groups receive the
// message exactly once (round-robin chosen by counter). Returns the number of
// distinct deliveries (for ErrNoResponders detection on Request).
func (s *Stub) deliver(msg *message, _ string) int {
	s.mu.RLock()
	matches := []*sub{}
	for pattern, list := range s.subs {
		if subjectMatches(pattern, msg.subject) {
			matches = append(matches, list...)
		}
	}
	s.mu.RUnlock()

	if len(matches) == 0 {
		return 0
	}

	// Group queue subs by queueGroup; one delivery per group.
	queues := map[string][]*sub{}
	plain := []*sub{}
	for _, sb := range matches {
		if sb.dead.Load() {
			continue
		}
		if sb.queueGroup == "" {
			plain = append(plain, sb)
		} else {
			queues[sb.queueGroup] = append(queues[sb.queueGroup], sb)
		}
	}

	count := 0
	for _, sb := range plain {
		s.dispatch(sb, msg)
		count++
	}
	for _, group := range queues {
		// Pick one round-robin via counter on the group's first member's pattern.
		// Simple approach: hash-step by an atomic counter on the stub.
		i := int(s.nextIndex()) % len(group)
		s.dispatch(group[i], msg)
		count++
	}
	return count
}

var roundRobinCounter atomic.Uint64

func (s *Stub) nextIndex() uint64 { return roundRobinCounter.Add(1) }

func (s *Stub) dispatch(sb *sub, msg *message) {
	sb.inFlight.Add(1)
	go func() {
		defer sb.inFlight.Done()
		// Synchronous handler in goroutine — mirrors NATS' goroutine-per-msg
		// (loose) behaviour but fully ordered per-subject is NOT guaranteed
		// by the stub (production NATS isn't either, modulo per-connection
		// in-order delivery).
		sb.handler(context.Background(), msg)
	}()
}

// ----------------------------------------------------------------------------
// Subject matching (NATS-style * and >)
// ----------------------------------------------------------------------------

// subjectMatches reports whether pattern matches subject under NATS rules:
//   - tokens delimited by "."
//   - "*" matches one token
//   - ">" matches one or more trailing tokens (must be the last token)
func subjectMatches(pattern, subject string) bool {
	pTokens := strings.Split(pattern, ".")
	sTokens := strings.Split(subject, ".")

	for i, p := range pTokens {
		if p == ">" {
			// Match all remaining tokens; > must consume at least one token.
			return i < len(sTokens)
		}
		if i >= len(sTokens) {
			return false
		}
		if p == "*" {
			continue
		}
		if p != sTokens[i] {
			return false
		}
	}
	return len(pTokens) == len(sTokens)
}

// ----------------------------------------------------------------------------
// Subscription handle
// ----------------------------------------------------------------------------

func (sb *sub) Drain() error {
	sb.dead.Store(true)
	sb.inFlight.Wait()
	sb.stub.removeSub(sb)
	return nil
}

func (sb *sub) Unsubscribe() error {
	sb.dead.Store(true)
	sb.stub.removeSub(sb)
	return nil
}

func (sb *sub) IsValid() bool { return !sb.dead.Load() }

// ----------------------------------------------------------------------------
// Message implementation
// ----------------------------------------------------------------------------

type message struct {
	subject     string
	header      mbapi.Header
	data        []byte
	reply       string
	respondFunc func(payload []byte) error
	streamSeq   uint64 // for stream messages
	streamAck   func() error
}

func (m *message) Subject() string      { return m.subject }
func (m *message) Header() mbapi.Header { return m.header }
func (m *message) Data() []byte         { return m.data }
func (m *message) Reply() string        { return m.reply }

func (m *message) Ack() error {
	if m.streamAck != nil {
		return m.streamAck()
	}
	return nil
}

func (m *message) Respond(payload []byte) error {
	if m.respondFunc == nil {
		return errors.New("stub: Respond on non-request message")
	}
	return m.respondFunc(payload)
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func timeoutFromCtx(ctx context.Context, optTimeout, fallback time.Duration) time.Duration {
	if optTimeout > 0 {
		return optTimeout
	}
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining > 0 {
			return remaining
		}
	}
	return fallback
}

var inboxCounter atomic.Uint64

func atomicID() string {
	n := inboxCounter.Add(1)
	return formatHex(n)
}

func formatHex(n uint64) string {
	const digits = "0123456789abcdef"
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n&0xf]
		n >>= 4
	}
	return string(buf[i:])
}

// Compile-time assertion that *Stub satisfies mbapi.MessageBus.
var _ mbapi.MessageBus = (*Stub)(nil)
