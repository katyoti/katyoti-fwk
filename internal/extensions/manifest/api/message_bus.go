// SPDX-License-Identifier: Apache-2.0

// This file defines the MessageBus interface — the only access path
// to NATS for application code.
//
// Per rule (import-discipline `nats-message-bus-only`): only the NATS transport package
// (internal/kts/transport/nats/) may import the NATS Go client directly.
// Every other package goes through MessageBus.
//
// Per ADR (modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md): this interface is NATS-shaped (subjects
// queue groups, JetStream, KV). It is not a lowest-common-denominator
// abstraction across arbitrary backends — production is always NATS.
//
// Test code uses an in-memory MessageBus stub from
// internal/extensions/manifest/teststub/. No embedded NATS server in unit tests.
//
// (The package documentation is doc.go.)

package api

import (
	"context"
	"time"
)

// Handler processes one inbound message. Implementations should be fast or
// offload work to a goroutine pool — slow handlers cause back-pressure.
type Handler func(ctx context.Context, msg Message)

// Message is one inbound payload. Producers fill Subject + Header + Data;
// consumers call Ack (for JetStream) or Respond (for req/rep).
type Message interface {
	Subject() string
	Header() Header
	Data() []byte
	Reply() string                // reply subject (auto-set for req/rep); empty otherwise
	Ack() error                   // for JetStream consumers
	Respond(payload []byte) error // for req/rep handlers
}

// Header carries request metadata. `meta.tenantId`, trace context, etc. ride here.
type Header map[string][]string

// Subscription is a handle to an active subscription. Drain for graceful
// unsubscribe (in-flight handlers complete); Unsubscribe for immediate.
type Subscription interface {
	Drain() error
	Unsubscribe() error
	IsValid() bool
}

// MessageBus is the application-facing interface. Production: NATS-backed.
// Tests: in-memory MessageBusStub.
type MessageBus interface {
	// Publish sends a fire-and-forget message on the given subject.
	// Returns an error if the local connection is dead; does NOT confirm delivery.
	Publish(ctx context.Context, subject string, payload []byte, opts ...PubOpt) error

	// Subscribe installs a handler that receives every message on the subject.
	// All subscribers see all messages — no load balancing.
	Subscribe(ctx context.Context, subject string, handler Handler, opts ...SubOpt) (Subscription, error)

	// QueueSubscribe installs a handler in a queue group. NATS round-robins
	// each message to ONE subscriber in the group across all replicas.
	// Used by sidecar slot handlers (queueGroup = "slot:<slot-name>").
	QueueSubscribe(ctx context.Context, subject, queueGroup string, handler Handler, opts ...SubOpt) (Subscription, error)

	// Request sends a request and waits for one reply. Timeout via ctx.
	// Returns ErrNoResponders if no subscriber is listening.
	Request(ctx context.Context, subject string, payload []byte, opts ...ReqOpt) ([]byte, error)

	// JetStream returns the JetStream sub-context (streams + KV).
	JetStream() JetStreamContext

	// Drain flushes in-flight subscriptions and closes the connection cleanly.
	// Use on graceful shutdown.
	Drain(ctx context.Context) error
}

// JetStreamContext exposes streams and KV buckets. Stream + bucket names
// are constants in internal/kts/transport/nats/embedded/streams.go — do not construct
// dynamically.
type JetStreamContext interface {
	Stream(name string) StreamContext
	KV(bucket string) KVContext
}

// StreamContext provides access to one append-only stream.
type StreamContext interface {
	Publish(ctx context.Context, subject string, payload []byte, opts ...JsPubOpt) (PubAck, error)
	PullConsumer(ctx context.Context, name string, cfg ConsumerConfig) (PullConsumer, error)
}

// KVContext provides typed access to one JetStream KV bucket.
type KVContext interface {
	Get(ctx context.Context, key string) (KVEntry, error)
	Put(ctx context.Context, key string, value []byte) (revision uint64, err error)
	PutWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) (revision uint64, err error)
	Delete(ctx context.Context, key string) error
	Watch(ctx context.Context, key string) (Watcher, error)
	UpdateCAS(ctx context.Context, key string, value []byte, expectedRevision uint64) (revision uint64, err error)
}

// KVEntry is one revision of a KV value.
type KVEntry interface {
	Key() string
	Value() []byte
	Revision() uint64
	Created() time.Time
}

// Watcher streams updates for one key (or one wildcard pattern in the future).
// Updates() emits the current value first, then each subsequent change.
type Watcher interface {
	Updates() <-chan KVEntry
	Stop() error
}

// PubAck confirms a JetStream stream publish.
type PubAck interface {
	Stream() string
	Sequence() uint64
}

// PullConsumer fetches messages from a stream in batches.
type PullConsumer interface {
	Fetch(ctx context.Context, batch int) ([]Message, error)
	Close() error
}

// ConsumerConfig parameterizes a pull consumer.
type ConsumerConfig struct {
	FilterSubject string
	DeliverPolicy DeliverPolicy
	AckPolicy     AckPolicy
}

type DeliverPolicy int

const (
	DeliverFromNew DeliverPolicy = iota
	DeliverFromStart
	DeliverFromTime // requires StartTime in opts (v2)
)

type AckPolicy int

const (
	AckExplicit AckPolicy = iota
	AckNone
)

// Option types — opaque to callers, decoded by the NATS adapter (and the stub).

type PubOpt func(*pubOptions)
type SubOpt func(*subOptions)
type ReqOpt func(*reqOptions)
type JsPubOpt func(*jsPubOptions)

type pubOptions struct {
	header Header
}

type subOptions struct {
	maxInFlight int
}

type reqOptions struct {
	timeout time.Duration
	header  Header
}

type jsPubOptions struct {
	header Header
	msgID  string // for dedup
}

// WithHeader attaches headers to a publish or request.
func WithHeader(h Header) PubOpt { return func(o *pubOptions) { o.header = h } }

// WithMaxInFlight caps the number of unprocessed messages a subscriber
// holds before back-pressure kicks in.
func WithMaxInFlight(n int) SubOpt { return func(o *subOptions) { o.maxInFlight = n } }

// WithRequestTimeout overrides the default request timeout for one call.
// (Default is taken from ctx; this overrides explicitly.)
func WithRequestTimeout(d time.Duration) ReqOpt { return func(o *reqOptions) { o.timeout = d } }

// WithRequestHeader attaches headers to a request.
func WithRequestHeader(h Header) ReqOpt { return func(o *reqOptions) { o.header = h } }

// WithMsgID sets a dedup ID for JetStream stream publishes (server-side
// duplicate detection within the dedup window).
func WithMsgID(id string) JsPubOpt { return func(o *jsPubOptions) { o.msgID = id } }

// ----------------------------------------------------------------------------
// Option extractors — for adapter packages to read option values.
// The option struct types are private; these helpers apply the funcs and
// return a public snapshot.
// ----------------------------------------------------------------------------

// ExtractedPubOptions is the public shape of accumulated PubOpts.
type ExtractedPubOptions struct {
	Header Header
}

// ExtractPubOpts applies all PubOpt closures and returns a public snapshot.
// Adapters call this to read option values without accessing private fields.
func ExtractPubOpts(opts []PubOpt) ExtractedPubOptions {
	var p pubOptions
	for _, o := range opts {
		o(&p)
	}
	return ExtractedPubOptions{Header: p.header}
}

// ExtractedSubOptions is the public shape of accumulated SubOpts.
type ExtractedSubOptions struct {
	MaxInFlight int
}

// ExtractSubOpts applies all SubOpt closures and returns a public snapshot.
func ExtractSubOpts(opts []SubOpt) ExtractedSubOptions {
	var s subOptions
	for _, o := range opts {
		o(&s)
	}
	return ExtractedSubOptions{MaxInFlight: s.maxInFlight}
}

// ExtractedReqOptions is the public shape of accumulated ReqOpts.
type ExtractedReqOptions struct {
	Timeout time.Duration
	Header  Header
}

// ExtractReqOpts applies all ReqOpt closures and returns a public snapshot.
func ExtractReqOpts(opts []ReqOpt) ExtractedReqOptions {
	var r reqOptions
	for _, o := range opts {
		o(&r)
	}
	return ExtractedReqOptions{Timeout: r.timeout, Header: r.header}
}

// ExtractedJsPubOptions is the public shape of accumulated JsPubOpts.
type ExtractedJsPubOptions struct {
	Header Header
	MsgID  string
}

// ExtractJsPubOpts applies all JsPubOpt closures and returns a public snapshot.
func ExtractJsPubOpts(opts []JsPubOpt) ExtractedJsPubOptions {
	var j jsPubOptions
	for _, o := range opts {
		o(&j)
	}
	return ExtractedJsPubOptions{Header: j.header, MsgID: j.msgID}
}

// ----------------------------------------------------------------------------
// Errors
// ----------------------------------------------------------------------------

// ErrNoResponders is returned by Request when no subscriber is listening on
// the subject.
type ErrNoResponders struct{ Subject string }

func (e ErrNoResponders) Error() string { return "no responders for subject: " + e.Subject }

// ErrTimeout is returned by Request when no reply arrives before the deadline.
type ErrTimeout struct{ Subject string }

func (e ErrTimeout) Error() string { return "request timeout: " + e.Subject }

// ErrSlowConsumer is delivered via the subscription's drop callback when
// the application can't keep up and NATS drops messages.
type ErrSlowConsumer struct{ Subject string }

func (e ErrSlowConsumer) Error() string { return "slow consumer: " + e.Subject }

// ErrSubjectNotPermitted is returned when the connection's account permissions
// forbid the requested subject (cloud / per-tenant).
type ErrSubjectNotPermitted struct{ Subject string }

func (e ErrSubjectNotPermitted) Error() string { return "subject not permitted: " + e.Subject }
