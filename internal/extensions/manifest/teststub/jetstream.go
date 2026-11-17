// SPDX-License-Identifier: Apache-2.0

package teststub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// ----------------------------------------------------------------------------
// JetStream context — entry point for streams + KV
// ----------------------------------------------------------------------------

type jsContext struct{ stub *Stub }

func (j *jsContext) Stream(name string) mbapi.StreamContext {
	j.stub.mu.Lock()
	defer j.stub.mu.Unlock()
	st, ok := j.stub.streams[name]
	if !ok {
		st = &memStream{name: name}
		j.stub.streams[name] = st
	}
	return &streamContext{stream: st}
}

func (j *jsContext) KV(bucket string) mbapi.KVContext {
	j.stub.mu.Lock()
	defer j.stub.mu.Unlock()
	b, ok := j.stub.buckets[bucket]
	if !ok {
		b = newMemKV(bucket)
		j.stub.buckets[bucket] = b
	}
	return &kvContext{kv: b}
}

// ----------------------------------------------------------------------------
// In-memory stream — append-only, with subject filtering for pull consumers
// ----------------------------------------------------------------------------

type memStream struct {
	name string
	mu   sync.Mutex
	seq  uint64
	msgs []*streamEntry
}

type streamEntry struct {
	seq     uint64
	subject string
	data    []byte
	header  mbapi.Header
}

type streamContext struct{ stream *memStream }

func (s *streamContext) Publish(ctx context.Context, subject string, payload []byte, opts ...mbapi.JsPubOpt) (mbapi.PubAck, error) {
	s.stream.mu.Lock()
	defer s.stream.mu.Unlock()
	s.stream.seq++
	e := &streamEntry{seq: s.stream.seq, subject: subject, data: payload}
	po := mbapi.ExtractJsPubOpts(opts)
	if len(po.Header) > 0 {
		e.header = po.Header
	}
	s.stream.msgs = append(s.stream.msgs, e)
	return &pubAck{stream: s.stream.name, seq: s.stream.seq}, nil
}

func (s *streamContext) PullConsumer(ctx context.Context, name string, cfg mbapi.ConsumerConfig) (mbapi.PullConsumer, error) {
	c := &pullConsumer{stream: s.stream, filter: cfg.FilterSubject}
	if cfg.DeliverPolicy == mbapi.DeliverFromNew {
		// Skip everything currently in the stream.
		s.stream.mu.Lock()
		c.cursor = uint64(len(s.stream.msgs))
		s.stream.mu.Unlock()
	}
	return c, nil
}

type pubAck struct {
	stream string
	seq    uint64
}

func (p *pubAck) Stream() string   { return p.stream }
func (p *pubAck) Sequence() uint64 { return p.seq }

type pullConsumer struct {
	stream *memStream
	filter string // subject filter (NATS wildcards)
	cursor uint64 // index into stream.msgs (0-based)
	closed atomic.Bool
}

func (p *pullConsumer) Fetch(ctx context.Context, batch int) ([]mbapi.Message, error) {
	if p.closed.Load() {
		return nil, errors.New("stub: pull consumer closed")
	}
	deadline := timeoutFromCtx(ctx, 0, 100*time.Millisecond)
	end := time.Now().Add(deadline)

	for {
		p.stream.mu.Lock()
		out := []mbapi.Message{}
		for int(p.cursor) < len(p.stream.msgs) && len(out) < batch {
			e := p.stream.msgs[p.cursor]
			p.cursor++
			if p.filter == "" || subjectMatches(p.filter, e.subject) {
				out = append(out, &message{
					subject: e.subject,
					data:    e.data,
					header:  e.header,
				})
			}
		}
		p.stream.mu.Unlock()
		if len(out) > 0 {
			return out, nil
		}
		if time.Now().After(end) {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (p *pullConsumer) Close() error {
	p.closed.Store(true)
	return nil
}

// ----------------------------------------------------------------------------
// In-memory KV bucket
// ----------------------------------------------------------------------------

type memKV struct {
	bucket   string
	mu       sync.RWMutex
	data     map[string]*kvRecord
	watchers map[string][]chan mbapi.KVEntry
}

type kvRecord struct {
	key      string
	value    []byte
	revision uint64
	created  time.Time
}

func newMemKV(name string) *memKV {
	return &memKV{
		bucket:   name,
		data:     map[string]*kvRecord{},
		watchers: map[string][]chan mbapi.KVEntry{},
	}
}

type kvContext struct{ kv *memKV }

func (k *kvContext) Get(_ context.Context, key string) (mbapi.KVEntry, error) {
	k.kv.mu.RLock()
	defer k.kv.mu.RUnlock()
	r, ok := k.kv.data[key]
	if !ok {
		return nil, fmt.Errorf("stub: key %q not found in bucket %s", key, k.kv.bucket)
	}
	return &kvEntry{r: r}, nil
}

func (k *kvContext) Put(_ context.Context, key string, value []byte) (uint64, error) {
	k.kv.mu.Lock()
	defer k.kv.mu.Unlock()
	r, ok := k.kv.data[key]
	if !ok {
		r = &kvRecord{key: key}
		k.kv.data[key] = r
	}
	r.value = append(r.value[:0], value...)
	r.revision++
	r.created = time.Now()
	k.kv.notify(key, r)
	return r.revision, nil
}

func (k *kvContext) PutWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) (uint64, error) {
	// Stub does not implement TTL — treat as a regular Put.
	_ = ttl
	return k.Put(ctx, key, value)
}

func (k *kvContext) Delete(_ context.Context, key string) error {
	k.kv.mu.Lock()
	defer k.kv.mu.Unlock()
	delete(k.kv.data, key)
	return nil
}

func (k *kvContext) Watch(_ context.Context, key string) (mbapi.Watcher, error) {
	ch := make(chan mbapi.KVEntry, 8)
	k.kv.mu.Lock()
	k.kv.watchers[key] = append(k.kv.watchers[key], ch)
	// Send current value if present.
	if r, ok := k.kv.data[key]; ok {
		select {
		case ch <- &kvEntry{r: r}:
		default:
		}
	}
	k.kv.mu.Unlock()
	return &kvWatcher{kv: k.kv, key: key, ch: ch}, nil
}

func (k *kvContext) UpdateCAS(_ context.Context, key string, value []byte, expectedRevision uint64) (uint64, error) {
	k.kv.mu.Lock()
	defer k.kv.mu.Unlock()
	r, ok := k.kv.data[key]
	if !ok {
		return 0, fmt.Errorf("stub: key %q not found", key)
	}
	if r.revision != expectedRevision {
		return 0, fmt.Errorf("stub: revision mismatch (have %d, expected %d)", r.revision, expectedRevision)
	}
	r.value = append(r.value[:0], value...)
	r.revision++
	r.created = time.Now()
	k.kv.notify(key, r)
	return r.revision, nil
}

func (k *memKV) notify(key string, r *kvRecord) {
	for _, ch := range k.watchers[key] {
		select {
		case ch <- &kvEntry{r: r}:
		default:
			// Slow watcher; drop.
		}
	}
}

type kvEntry struct{ r *kvRecord }

func (e *kvEntry) Key() string        { return e.r.key }
func (e *kvEntry) Value() []byte      { return e.r.value }
func (e *kvEntry) Revision() uint64   { return e.r.revision }
func (e *kvEntry) Created() time.Time { return e.r.created }

type kvWatcher struct {
	kv      *memKV
	key     string
	ch      chan mbapi.KVEntry
	stopped atomic.Bool
}

func (w *kvWatcher) Updates() <-chan mbapi.KVEntry { return w.ch }

func (w *kvWatcher) Stop() error {
	if w.stopped.Swap(true) {
		return nil
	}
	w.kv.mu.Lock()
	defer w.kv.mu.Unlock()
	list := w.kv.watchers[w.key]
	for i, ch := range list {
		if ch == w.ch {
			w.kv.watchers[w.key] = append(list[:i], list[i+1:]...)
			break
		}
	}
	close(w.ch)
	return nil
}
