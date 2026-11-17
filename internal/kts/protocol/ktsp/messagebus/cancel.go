// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// Cancelling a call.
//
// Every Call carries a fresh id in the HeaderCallID header. When the caller's
// context ends before the answer — it was cancelled, or its deadline passed —
// the Sender publishes the id on the provider's cancel subject
// (subjects.SlotCancelSubject). Subscribe listens there on every replica (a
// plain subscription: the caller does not know which replica of the queue
// group serves the call) and cancels the context of the call it serves under
// that id, so the method stops its work. A cancel that arrives before its call
// is dispatched (the call still waiting for a worker) is remembered for a
// short while and the call starts already cancelled.

// HeaderCallID carries the id of one KTSP call, the key its cancel names.
const HeaderCallID = "ktw.meta.callId"

// cancelNotice is the payload published on the cancel subject.
type cancelNotice struct {
	CallID string `json:"call_id"`
}

// earlyCancelTTL is how long a cancel for a call not yet dispatched is kept.
const earlyCancelTTL = time.Minute

// cancelPublishTimeout bounds the publish of a cancel notice, which runs
// apart from the call: the caller never waits for it.
const cancelPublishTimeout = 2 * time.Second

// newCallID is a random id for one call.
func newCallID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

// withCallID is a copy of h carrying the call's id; h is never written to.
func withCallID(h mbapi.Header, id string) mbapi.Header {
	out := make(mbapi.Header, len(h)+1)
	for k, v := range h {
		out[k] = v
	}
	out[HeaderCallID] = []string{id}
	return out
}

// CallIDFromMessage is the call id an inbound request carries, "" when none.
func CallIDFromMessage(msg mbapi.Message) string {
	if h := msg.Header(); h != nil {
		if v := h[HeaderCallID]; len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// sendCancel tells the provider's replicas that the call id is no longer
// needed. Best effort, and run apart from the call: the caller has its error
// at once, and a notice that cannot leave changes nothing for it.
func (s *Sender) sendCancel(provider, id string) {
	if id == "" {
		return
	}
	body, err := json.Marshal(cancelNotice{CallID: id})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cancelPublishTimeout)
	defer cancel()
	_ = s.bus.Publish(ctx, subjects.SlotCancelSubject(s.tenantID, s.universeID, provider), body,
		mbapi.WithHeader(s.header))
}

// callRegistry holds the cancel function of every call a subscription serves,
// by call id, and the cancels that came before their call.
type callRegistry struct {
	mu      sync.Mutex
	running map[string]context.CancelFunc
	early   map[string]time.Time
	now     func() time.Time
}

func newCallRegistry() *callRegistry {
	return &callRegistry{
		running: map[string]context.CancelFunc{},
		early:   map[string]time.Time{},
		now:     time.Now,
	}
}

// begin derives the context a call is served under; done must be called when
// the call ends. A call whose cancel already came starts cancelled.
func (r *callRegistry) begin(ctx context.Context, id string) (context.Context, func()) {
	c, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.early[id]; ok {
		delete(r.early, id)
		cancel()
		return c, func() {}
	}
	r.running[id] = cancel
	return c, func() {
		r.mu.Lock()
		delete(r.running, id)
		r.mu.Unlock()
		cancel()
	}
}

// cancel ends the call served under id, or remembers the cancel until the
// call is dispatched.
func (r *callRegistry) cancel(id string) {
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.running[id]; ok {
		delete(r.running, id)
		f()
		return
	}
	now := r.now()
	for k, at := range r.early {
		if now.Sub(at) > earlyCancelTTL {
			delete(r.early, k)
		}
	}
	r.early[id] = now
}

// cancelHandler serves the cancel subject: each notice cancels its call.
func (r *callRegistry) cancelHandler() mbapi.Handler {
	return func(_ context.Context, msg mbapi.Message) {
		var n cancelNotice
		if err := json.Unmarshal(msg.Data(), &n); err != nil {
			return
		}
		r.cancel(n.CallID)
	}
}
