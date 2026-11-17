// SPDX-License-Identifier: Apache-2.0

package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	natsgo "github.com/nats-io/nats.go"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/busauth"
)

// Adapter wraps a *nats.Conn to satisfy mbapi.MessageBus.
//
// A keyed adapter (NewKeyed) signs every message it sends with its program's
// key and tells the sender of every message it receives (busauth.Stamped)
// from the keys folder's registry; an adapter without a key does neither, and
// its messages answer busauth.ErrOpenBus.
type Adapter struct {
	conn *natsgo.Conn
	cred *busauth.Credential
	reg  busauth.Lookup
}

// New wraps a NATS connection. The connection must already be established.
//
// Hub-side callers that have an `*embedded.EmbeddedHandle` from
// `embedded.StartEmbedded` can use this directly:
//
//	adapter := nats.New(h.Client)
func New(conn *natsgo.Conn) *Adapter {
	return &Adapter{conn: conn}
}

// NewKeyed wraps a NATS connection made with cred: what it sends is signed
// with cred, what it receives is checked against cred's registry. A nil cred
// is New.
func NewKeyed(conn *natsgo.Conn, cred *busauth.Credential) *Adapter {
	if cred == nil {
		return New(conn)
	}
	return &Adapter{conn: conn, cred: cred, reg: cred.Registry()}
}

// signed is the header a message of subject and payload goes out with: the
// caller's, signed when the adapter is keyed (a copy - the caller's map is
// never written to).
func (a *Adapter) signed(subject string, h mbapi.Header, payload []byte) (natsgo.Header, error) {
	if a.cred == nil {
		return natsgo.Header(h), nil
	}
	out := make(map[string][]string, len(h)+4)
	for k, v := range h {
		out[k] = v
	}
	signed, err := a.cred.Sign(subject, out, payload, time.Now())
	if err != nil {
		return nil, err
	}
	return natsgo.Header(signed), nil
}

// ----------------------------------------------------------------------------
// Publish / Subscribe / QueueSubscribe / Request
// ----------------------------------------------------------------------------

func (a *Adapter) Publish(ctx context.Context, subject string, payload []byte, opts ...mbapi.PubOpt) error {
	po := mbapi.ExtractPubOpts(opts)
	if len(po.Header) == 0 && a.cred == nil {
		return a.conn.Publish(subject, payload)
	}
	h, err := a.signed(subject, po.Header, payload)
	if err != nil {
		return err
	}
	msg := &natsgo.Msg{Subject: subject, Data: payload, Header: h}
	return a.conn.PublishMsg(msg)
}

func (a *Adapter) Subscribe(ctx context.Context, subject string, handler mbapi.Handler, opts ...mbapi.SubOpt) (mbapi.Subscription, error) {
	sub, err := a.conn.Subscribe(subject, a.wrapHandler(handler))
	if err != nil {
		return nil, err
	}
	return &subscription{sub: sub}, nil
}

func (a *Adapter) QueueSubscribe(ctx context.Context, subject, queueGroup string, handler mbapi.Handler, opts ...mbapi.SubOpt) (mbapi.Subscription, error) {
	sub, err := a.conn.QueueSubscribe(subject, queueGroup, a.wrapHandler(handler))
	if err != nil {
		return nil, err
	}
	return &subscription{sub: sub}, nil
}

func (a *Adapter) Request(ctx context.Context, subject string, payload []byte, opts ...mbapi.ReqOpt) ([]byte, error) {
	ro := mbapi.ExtractReqOpts(opts)
	timeout := timeoutFromCtx(ctx, ro.Timeout, 5*time.Second)

	if len(ro.Header) > 0 || a.cred != nil {
		h, err := a.signed(subject, ro.Header, payload)
		if err != nil {
			return nil, err
		}
		msg, err := a.conn.RequestMsg(&natsgo.Msg{Subject: subject, Data: payload, Header: h}, timeout)
		if err != nil {
			return nil, mapRequestErr(subject, err)
		}
		return msg.Data, nil
	}
	msg, err := a.conn.Request(subject, payload, timeout)
	if err != nil {
		return nil, mapRequestErr(subject, err)
	}
	return msg.Data, nil
}

// ----------------------------------------------------------------------------
// Lifecycle
// ----------------------------------------------------------------------------

func (a *Adapter) Drain(ctx context.Context) error {
	if err := a.conn.Drain(); err != nil {
		return fmt.Errorf("drain: %w", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && a.conn.Status() != natsgo.CLOSED {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// JetStream
// ----------------------------------------------------------------------------

func (a *Adapter) JetStream() mbapi.JetStreamContext {
	js, err := a.conn.JetStream()
	if err != nil {
		// Should not happen for a healthy connection; embed the error so the
		// first call from the wrapper surfaces it.
		return &jsContext{err: err}
	}
	return &jsContext{js: js}
}

type jsContext struct {
	js  natsgo.JetStreamContext
	err error
}

func (j *jsContext) Stream(name string) mbapi.StreamContext {
	if j.err != nil {
		return &streamContext{err: j.err}
	}
	return &streamContext{js: j.js, name: name}
}

func (j *jsContext) KV(bucket string) mbapi.KVContext {
	if j.err != nil {
		return &kvContext{err: j.err}
	}
	kv, err := j.js.KeyValue(bucket)
	if err != nil {
		return &kvContext{err: fmt.Errorf("KV %s: %w", bucket, err)}
	}
	return &kvContext{kv: kv}
}

// ---- Stream ----

type streamContext struct {
	js   natsgo.JetStreamContext
	name string
	err  error
}

func (s *streamContext) Publish(ctx context.Context, subject string, payload []byte, opts ...mbapi.JsPubOpt) (mbapi.PubAck, error) {
	if s.err != nil {
		return nil, s.err
	}
	po := mbapi.ExtractJsPubOpts(opts)
	publishOpts := []natsgo.PubOpt{}
	if po.MsgID != "" {
		publishOpts = append(publishOpts, natsgo.MsgId(po.MsgID))
	}
	natsAck, err := s.js.Publish(subject, payload, publishOpts...)
	if err != nil {
		return nil, err
	}
	return &pubAck{ack: natsAck}, nil
}

func (s *streamContext) PullConsumer(ctx context.Context, name string, cfg mbapi.ConsumerConfig) (mbapi.PullConsumer, error) {
	if s.err != nil {
		return nil, s.err
	}
	natsCfg := &natsgo.ConsumerConfig{
		Durable:       name,
		FilterSubject: cfg.FilterSubject,
		AckPolicy:     mapAckPolicy(cfg.AckPolicy),
		DeliverPolicy: mapDeliverPolicy(cfg.DeliverPolicy),
	}
	if _, err := s.js.AddConsumer(s.name, natsCfg); err != nil && !errors.Is(err, natsgo.ErrConsumerNameAlreadyInUse) {
		return nil, fmt.Errorf("AddConsumer %s/%s: %w", s.name, name, err)
	}
	sub, err := s.js.PullSubscribe(cfg.FilterSubject, name, natsgo.Bind(s.name, name))
	if err != nil {
		return nil, fmt.Errorf("PullSubscribe %s/%s: %w", s.name, name, err)
	}
	return &pullConsumer{sub: sub}, nil
}

type pubAck struct{ ack *natsgo.PubAck }

func (p *pubAck) Stream() string   { return p.ack.Stream }
func (p *pubAck) Sequence() uint64 { return p.ack.Sequence }

type pullConsumer struct{ sub *natsgo.Subscription }

func (p *pullConsumer) Fetch(ctx context.Context, batch int) ([]mbapi.Message, error) {
	timeout := timeoutFromCtx(ctx, 0, 5*time.Second)
	natsMsgs, err := p.sub.Fetch(batch, natsgo.MaxWait(timeout))
	if err != nil {
		return nil, err
	}
	out := make([]mbapi.Message, 0, len(natsMsgs))
	for _, m := range natsMsgs {
		out = append(out, &message{msg: m})
	}
	return out, nil
}

func (p *pullConsumer) Close() error { return p.sub.Drain() }

// ---- KV ----

type kvContext struct {
	kv  natsgo.KeyValue
	err error
}

func (k *kvContext) Get(ctx context.Context, key string) (mbapi.KVEntry, error) {
	if k.err != nil {
		return nil, k.err
	}
	e, err := k.kv.Get(key)
	if err != nil {
		return nil, err
	}
	return &kvEntry{e: e}, nil
}

func (k *kvContext) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	if k.err != nil {
		return 0, k.err
	}
	return k.kv.Put(key, value)
}

func (k *kvContext) PutWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) (uint64, error) {
	// JetStream KV TTL is per-bucket, not per-key (in current nats.go).
	// We honour bucket-level TTL configured at provisioning time
	// (e.g. KTW_PRESENCE has TTL 30s). The ttl arg here is informational.
	_ = ttl
	return k.Put(ctx, key, value)
}

func (k *kvContext) Delete(ctx context.Context, key string) error {
	if k.err != nil {
		return k.err
	}
	return k.kv.Delete(key)
}

func (k *kvContext) Watch(ctx context.Context, key string) (mbapi.Watcher, error) {
	if k.err != nil {
		return nil, k.err
	}
	w, err := k.kv.Watch(key)
	if err != nil {
		return nil, err
	}
	return &kvWatcher{w: w}, nil
}

func (k *kvContext) UpdateCAS(ctx context.Context, key string, value []byte, expectedRevision uint64) (uint64, error) {
	if k.err != nil {
		return 0, k.err
	}
	return k.kv.Update(key, value, expectedRevision)
}

type kvEntry struct{ e natsgo.KeyValueEntry }

func (e *kvEntry) Key() string        { return e.e.Key() }
func (e *kvEntry) Value() []byte      { return e.e.Value() }
func (e *kvEntry) Revision() uint64   { return e.e.Revision() }
func (e *kvEntry) Created() time.Time { return e.e.Created() }

type kvWatcher struct {
	w   natsgo.KeyWatcher
	out chan mbapi.KVEntry
}

func (w *kvWatcher) Updates() <-chan mbapi.KVEntry {
	if w.out == nil {
		w.out = make(chan mbapi.KVEntry, 8)
		go func() {
			defer close(w.out)
			for upd := range w.w.Updates() {
				if upd == nil {
					// nats.go signals end-of-history with nil; we still
					// keep streaming live updates.
					continue
				}
				w.out <- &kvEntry{e: upd}
			}
		}()
	}
	return w.out
}

func (w *kvWatcher) Stop() error { return w.w.Stop() }

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

type subscription struct{ sub *natsgo.Subscription }

func (s *subscription) Drain() error       { return s.sub.Drain() }
func (s *subscription) Unsubscribe() error { return s.sub.Unsubscribe() }
func (s *subscription) IsValid() bool      { return s.sub.IsValid() }

// message is a received message; reg is the registry of a keyed adapter, nil
// on an open bus.
type message struct {
	msg *natsgo.Msg
	reg busauth.Lookup
}

// Caller implements busauth.Stamped: whose key signed the message.
func (m *message) Caller() (busauth.Caller, error) {
	if m.reg == nil {
		return busauth.Caller{}, busauth.ErrOpenBus
	}
	return busauth.Verify(m.reg, m.msg.Subject, m.msg.Header, m.msg.Data, time.Now())
}

// CauseAttester implements busauth.Stamped: whose key attested the cause.
func (m *message) CauseAttester() (busauth.Caller, error) {
	if m.reg == nil {
		return busauth.Caller{}, busauth.ErrOpenBus
	}
	return busauth.VerifyCause(m.reg, m.msg.Header, time.Now())
}

func (m *message) Subject() string { return m.msg.Subject }
func (m *message) Header() mbapi.Header {
	if m.msg.Header == nil {
		return nil
	}
	return mbapi.Header(m.msg.Header)
}
func (m *message) Data() []byte                 { return m.msg.Data }
func (m *message) Reply() string                { return m.msg.Reply }
func (m *message) Ack() error                   { return m.msg.Ack() }
func (m *message) Respond(payload []byte) error { return m.msg.Respond(payload) }

func (a *Adapter) wrapHandler(h mbapi.Handler) natsgo.MsgHandler {
	return func(natsMsg *natsgo.Msg) {
		// We don't have a per-message context from nats.go; use Background.
		// Callers that need cancellation install their own ctx around their handler.
		h(context.Background(), &message{msg: natsMsg, reg: a.reg})
	}
}

func mapRequestErr(subject string, err error) error {
	switch {
	case errors.Is(err, natsgo.ErrNoResponders):
		return mbapi.ErrNoResponders{Subject: subject}
	case errors.Is(err, natsgo.ErrTimeout):
		return mbapi.ErrTimeout{Subject: subject}
	default:
		return err
	}
}

func mapAckPolicy(p mbapi.AckPolicy) natsgo.AckPolicy {
	switch p {
	case mbapi.AckNone:
		return natsgo.AckNonePolicy
	default:
		return natsgo.AckExplicitPolicy
	}
}

func mapDeliverPolicy(p mbapi.DeliverPolicy) natsgo.DeliverPolicy {
	switch p {
	case mbapi.DeliverFromStart:
		return natsgo.DeliverAllPolicy
	case mbapi.DeliverFromTime:
		return natsgo.DeliverByStartTimePolicy
	default:
		return natsgo.DeliverNewPolicy
	}
}

// timeoutFromCtx returns optTimeout if non-zero, else the deadline from ctx,
// else fallback. NATS Go API requires an absolute timeout for Request/Fetch.
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
