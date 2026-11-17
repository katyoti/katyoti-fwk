// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// EventRaise records an event that is not an object write (a code change) on
// the object it concerns, as a write's event is recorded: stamped (a fresh
// event id, the time, the object's CURRENT revision — read, not changed — and
// the caller's cause), published (`object.<token>.event`, which the journal
// records), then its type's point fired on the object (fireAfterHooks): the
// async entries matched under the cascade guards (depth, no self-reaction,
// accepts_caused, coalescing) — staged in the outbox under a reaction's
// cause — and the plain on_* entries at that point journaled and run.
//
// Refused (ErrInvalidInput): no type, a type no event-type declares, one that
// names no point, CWM's own (object.created … are raised by its writes), no
// object, a payload that is not JSON. An unknown object is not found. No
// raise restriction: whoever reaches CWM raises any declared type.
func (s *Service) EventRaise(ctx stdctx.Context, in api.EventRaiseInput) (api.EventRaiseOutput, error) {
	et, err := s.raisableEventType(in.Type)
	if err != nil {
		return api.EventRaiseOutput{}, err
	}
	if in.ObjectID == "" {
		return api.EventRaiseOutput{}, fmt.Errorf("%w: EventRaise: object_id required", api.ErrInvalidInput)
	}
	payload, err := compactPayload(in.Payload)
	if err != nil {
		return api.EventRaiseOutput{}, err
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.EventRaiseOutput{}, err
	}
	module, ok := ix.moduleOf(in.ObjectID)
	if !ok {
		return api.EventRaiseOutput{}, api.ObjectNotFound(in.ObjectID)
	}
	got, err := s.ObjectGet(ctx, api.ObjectGetInput{Module: module, ID: in.ObjectID})
	if err != nil {
		return api.EventRaiseOutput{}, err
	}
	obj := got.Object
	meta := s.eventMeta(ctx, got.Revision)
	s.Events.PublishObjectEvent(api.ObjectRaisedEventPayload{
		EventMeta: meta, ObjectID: obj.ID, Module: module, Type: obj.Type, Event: et.Name,
		Summary: api.CutNote(string(payload)),
	})
	ev := api.MatchedEvent{EventMeta: meta, Action: api.RaisedEventAction, Type: obj.Type,
		ObjectID: obj.ID, Module: module, Event: et.Name, Payload: payload}
	s.fireAfterHooks(ctx, et.Point, obj, nil, ev)
	return api.EventRaiseOutput{EventMeta: meta}, nil
}

// raisableEventType is the declared event type name, refused when it is not
// one EventRaise may raise.
func (s *Service) raisableEventType(name string) (api.EventType, error) {
	if name == "" {
		return api.EventType{}, fmt.Errorf("%w: EventRaise: type required", api.ErrInvalidInput)
	}
	if api.IsBuiltinEventType(name) {
		return api.EventType{}, fmt.Errorf("%w: EventRaise: %s is CWM's own event, raised by its writes", api.ErrInvalidInput, name)
	}
	for _, t := range s.decl().EventTypes {
		if t.Name != name {
			continue
		}
		if t.Point == "" {
			return api.EventType{}, fmt.Errorf("%w: EventRaise: event type %s names no point — nothing could subscribe to it", api.ErrInvalidInput, name)
		}
		return t, nil
	}
	return api.EventType{}, fmt.Errorf("%w: EventRaise: event type %s is not declared (no event-type contribution names it)", api.ErrInvalidInput, name)
}

// compactPayload is a raised event's payload as compact JSON; empty stays
// empty, anything that is not JSON is refused.
func compactPayload(raw json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return nil, fmt.Errorf("%w: EventRaise: payload is not JSON: %v", api.ErrInvalidInput, err)
	}
	return buf.Bytes(), nil
}
