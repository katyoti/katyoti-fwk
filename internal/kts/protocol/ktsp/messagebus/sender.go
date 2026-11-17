// SPDX-License-Identifier: Apache-2.0

// Package messagebus is the KTSP transport over the MessageBus interface.
//
// Sender implements ktsp.Sender by translating Send/Notify into
// MessageBus.Request and MessageBus.Publish on the canonical slot
// subject (per (modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md)):
//
//	ktw.<tenant>.universe.<u>.slot.<slot>.<method>
//
// Subscribe (in subscribe.go) bridges from MessageBus subjects back
// into the local ktsp.Endpoint.Handle dispatch — the sidecar side.
//
// Wire protocol:
//   - Request body: the JSON-encoded params (raw, no envelope).
//   - Response body: `{"result": <raw>, "error": <RPCError or omitted>}`
//     envelope so success and error are always distinguishable. A
//     missing `result` field with no `error` means "ok, empty result".
//
// The wire-level retry + ID generation lives one layer above (the
// existing ktsp.RetrySender wraps any Transport). messagebus.Sender
// is itself a ktsp.Sender — it doesn't need a Transport and so doesn't
// pass through RetrySender's wrapping; if callers want retry, they
// can construct a parallel RetrySender that delegates to this Sender's
// underlying MessageBus.
package messagebus

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/busauth"
)

// Wire-level header keys carried on every messagebus request/notify.
// Per (modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md), these mirror NATS-convention
// `meta.*` fields. Subjects already carry tenant + universe in the
// path, but the headers carry the same identity in a stable place
// for downstream consumers (auth gates, trace correlation, audit
// logs) that don't want to parse subject strings.
const (
	HeaderTenantID   = "ktw.meta.tenantId"
	HeaderUniverseID = "ktw.meta.universeId"
	// HeaderCause carries the caller's cause (internal/core/cause, as JSON:
	// `{"kind","id","depth"}`) when its context holds one, so a write the
	// serving side makes for this call says why it was made. Absent when the
	// caller's context carries no cause.
	HeaderCause = busauth.HeaderCause
	// HeaderPanel carries the panel handle the caller's context lends to
	// the call (panel.WithHandle, as JSON: panel.Ref `{id, seq, tenant,
	// universe}`), so the method serving it reports into the caller's panel
	// and its answer gives the last sequence back (envelope.PanelSeq,
	// ). Absent when the caller's context lends none.
	HeaderPanel = "ktw.meta.panel"
)

// Sender routes ktsp.Send / Notify through a MessageBus to a sidecar
// listening on the canonical slot subject. Every outbound message
// carries `meta.tenantId` + `meta.universeId` headers so subscribers
// see the originator scope without having to parse the subject.
type Sender struct {
	bus        mbapi.MessageBus
	tenantID   string
	universeID string
	header     mbapi.Header // pre-built header re-used per call (read-only after construction)
}

// headerFor is the header one call carries: the fixed scope header, plus
// HeaderCause when ctx carries a cause and HeaderPanel when it lends a panel
// handle. The fixed header is never written to — a call with either gets its
// own copy.
func (s *Sender) headerFor(ctx context.Context) mbapi.Header {
	extra := map[string]string{}
	if c, ok := cause.CauseFrom(ctx); ok {
		if raw, err := json.Marshal(c); err == nil {
			extra[HeaderCause] = string(raw)
		}
		// The cause of the call being served, relayed: it goes on as it came,
		// with its person's attestation (busauth) - the very header signed.
		if raw, att, ok := busauth.CauseAttestationFrom(ctx); ok && sameCause(raw, c) {
			extra[HeaderCause] = raw
			extra[busauth.HeaderCauseSig] = att
		}
	}
	if ph, ok := panel.HandleFrom(ctx); ok {
		if raw, err := json.Marshal(ph.Ref()); err == nil {
			extra[HeaderPanel] = string(raw)
		}
	}
	if len(extra) == 0 {
		return s.header
	}
	h := make(mbapi.Header, len(s.header)+len(extra))
	for k, v := range s.header {
		h[k] = v
	}
	for k, v := range extra {
		h[k] = []string{v}
	}
	return h
}

// sameCause reports whether the cause header raw says c.
func sameCause(raw string, c cause.Cause) bool {
	var got cause.Cause
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return false
	}
	return reflect.DeepEqual(got, c)
}

// NewSender constructs a Sender bound to the given MessageBus and
// (tenant, universe) scope. Both IDs MUST satisfy the subject token
// charset — the subject builders panic on invalid tokens. The
// (tenant, universe) pair also becomes the value of the
// `meta.tenantId` / `meta.universeId` headers on every outbound call.
func NewSender(bus mbapi.MessageBus, tenantID, universeID string) *Sender {
	return &Sender{
		bus:        bus,
		tenantID:   tenantID,
		universeID: universeID,
		header: mbapi.Header{
			HeaderTenantID:   []string{tenantID},
			HeaderUniverseID: []string{universeID},
		},
	}
}

// Send implements ktsp.Sender. The path is an authoring slot URI per
// the grammar in path.go::ParsePath:
//
//	[PROVIDER]':/'TYPE['('INSTANCE')']'/'METHOD
//
// Empty provider (`":/Type/Method"`) resolves via ctx's self-provider
// (`application.SelfProviderFromContext`). Empty `(instance)` defaults
// to `"default"`. Type and method may be PascalCase OR kebab — they get
// kebab-normalized at parse time.
func (s *Sender) Call(ctx context.Context, path string, params any, opts ...ktsp.CallOption) (json.RawMessage, error) {
	if o := ktsp.ApplyCallOptions(opts...); o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	p, err := ParsePath(ctx, path)
	if err != nil {
		return nil, err
	}
	body, err := encodeParams(params)
	if err != nil {
		return nil, err
	}
	subject := subjects.SlotSubject(s.tenantID, s.universeID, p.Provider, p.Slottype, p.Instance, p.Method)
	// The call's id, so a caller that gives up can cancel it (cancel.go).
	callID := newCallID()
	// The request's id, the same on each retry of this logical call: the
	// serving side answers it once (replay.go).
	requestID := RequestIDFrom(ctx)
	if requestID == "" {
		requestID = newCallID()
	}
	h := withCallID(s.headerFor(ctx), callID)
	h[HeaderRequestID] = []string{requestID}
	resp, err := s.bus.Request(ctx, subject, body, mbapi.WithRequestHeader(h))
	if err != nil {
		if ctx.Err() != nil {
			// The caller no longer waits (cancelled, or past its deadline):
			// the method serving it stops too. The notice leaves on its own,
			// so the caller gets its error at once.
			go s.sendCancel(p.Provider, callID)
		}
		return nil, mapBusErr(p, err)
	}
	result, panelSeq, err := decodeResponseSeq(resp)
	// The panel the call was lent goes on after the last event its method
	// sent, answered or failed.
	if ph, ok := panel.HandleFrom(ctx); ok && panelSeq > 0 {
		ph.Advance(panelSeq)
	}
	return result, err
}

// Notify implements ktsp.Sender (fire-and-forget). Path grammar is the
// same as Call's.
func (s *Sender) Notify(ctx context.Context, path string, params any, opts ...ktsp.CallOption) error {
	if o := ktsp.ApplyCallOptions(opts...); o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	p, err := ParsePath(ctx, path)
	if err != nil {
		return err
	}
	body, err := encodeParams(params)
	if err != nil {
		return err
	}
	subject := subjects.SlotSubject(s.tenantID, s.universeID, p.Provider, p.Slottype, p.Instance, p.Method)
	return s.bus.Publish(ctx, subject, body, mbapi.WithHeader(s.headerFor(ctx)))
}

// HeadersFromMessage extracts the (tenantId, universeId) carried on a
// messagebus.Message. Subscribers / dispatched handlers call this when
// they need the originator scope. Empty strings on missing headers.
func HeadersFromMessage(msg mbapi.Message) (tenantID, universeID string) {
	h := msg.Header()
	if h == nil {
		return "", ""
	}
	if v := h[HeaderTenantID]; len(v) > 0 {
		tenantID = v[0]
	}
	if v := h[HeaderUniverseID]; len(v) > 0 {
		universeID = v[0]
	}
	return tenantID, universeID
}

// PanelFromMessage is the panel handle a call was lent (HeaderPanel), and
// whether it carries one; a header that does not decode or names no panel
// counts as none.
func PanelFromMessage(msg mbapi.Message) (panel.Ref, bool) {
	h := msg.Header()
	if h == nil {
		return panel.Ref{}, false
	}
	v := h[HeaderPanel]
	if len(v) == 0 || v[0] == "" {
		return panel.Ref{}, false
	}
	var r panel.Ref
	if err := json.Unmarshal([]byte(v[0]), &r); err != nil || r.Validate() != nil {
		return panel.Ref{}, false
	}
	return r, true
}

// CauseFromMessage is the cause a message carries (HeaderCause), and whether
// it carries one; a header that does not decode counts as none.
func CauseFromMessage(msg mbapi.Message) (cause.Cause, bool) {
	h := msg.Header()
	if h == nil {
		return cause.Cause{}, false
	}
	v := h[HeaderCause]
	if len(v) == 0 || v[0] == "" {
		return cause.Cause{}, false
	}
	var c cause.Cause
	if err := json.Unmarshal([]byte(v[0]), &c); err != nil || c.IsZero() {
		return cause.Cause{}, false
	}
	return c, true
}

// Compile-time interface assertion.
var _ ktsp.Sender = (*Sender)(nil)

// ----------------------------------------------------------------------------
// Wire helpers — shared by Sender and Subscribe.
// ----------------------------------------------------------------------------

// envelope is the response envelope: success = result populated, error
// nil; failure = error populated (result may be nil). PanelSeq is the last
// sequence number the method sent on the panel its call was lent
// (HeaderPanel), answered or failed. Every field is `omitempty` so the
// on-the-wire payload is minimal.
type envelope struct {
	Result   json.RawMessage `json:"result,omitempty"`
	Error    *ktsp.RPCError  `json:"error,omitempty"`
	PanelSeq int64           `json:"panelSeq,omitempty"`
}

func encodeParams(params any) ([]byte, error) {
	if params == nil {
		return []byte("null"), nil
	}
	if raw, ok := params.(json.RawMessage); ok {
		if len(raw) == 0 {
			return []byte("null"), nil
		}
		return []byte(raw), nil
	}
	return json.Marshal(params)
}

func encodeResponse(result json.RawMessage, rpcErr *ktsp.RPCError) ([]byte, error) {
	return encodeResponseSeq(result, rpcErr, 0)
}

// encodeResponseSeq is encodeResponse with the lent panel's last sequence.
func encodeResponseSeq(result json.RawMessage, rpcErr *ktsp.RPCError, panelSeq int64) ([]byte, error) {
	env := envelope{PanelSeq: panelSeq}
	if rpcErr != nil {
		env.Error = rpcErr
	} else {
		env.Result = result
	}
	return json.Marshal(env)
}

// decodeResponseSeq reads a response envelope: its result or its error, and
// the lent panel's last sequence.
func decodeResponseSeq(payload []byte) (json.RawMessage, int64, error) {
	if len(payload) == 0 {
		return nil, 0, nil
	}
	var env envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, 0, fmt.Errorf("messagebus: malformed response envelope: %w", err)
	}
	if env.Error != nil {
		return nil, env.PanelSeq, env.Error
	}
	return env.Result, env.PanelSeq, nil
}

// mapBusErr maps NATS-layer transport errors into the typed ktsp.RPCError
// shapes callers already handle. The error message embeds the parsed Path
// so logs identify the target as `provider:/slottype(instance)/method`
// rather than an opaque subject string.
func mapBusErr(p Path, err error) error {
	switch err.(type) {
	case mbapi.ErrNoResponders:
		return ktsp.ErrSlotNotFound(p.Subject())
	case mbapi.ErrTimeout:
		return ktsp.ErrInternalError("request timeout: " + p.Subject())
	default:
		return err
	}
}
