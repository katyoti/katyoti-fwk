// SPDX-License-Identifier: Apache-2.0

package events

import (
	"encoding/json"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Typed payloads emitted alongside CWM event subjects. They land
// in `api.EventEnvelope.Payload` as JSON; consumers unmarshal
// into the matching struct based on the subject's entity class.
//
// The payloads stay small: just enough to let a subscriber act
// without re-fetching the entity. Fuller details require a
// follow-up `Get` call.

// The payloads are the API's (api.*EventPayload) — a subscriber outside CWM
// decodes the same types.
type (
	ObjectPayload                  = api.ObjectEventPayload
	ObjectTransitionedPayload      = api.ObjectTransitionedEventPayload
	ObjectTransitionRefusedPayload = api.ObjectTransitionRefusedEventPayload
	ObjectMovedPayload             = api.ObjectMovedEventPayload
	ObjectSplitPayload             = api.ObjectSplitEventPayload
	ObjectArchivedPayload          = api.ObjectArchivedEventPayload
	ObjectsCaughtUpPayload         = api.ObjectsCaughtUpEventPayload
)

// payloadToRaw marshals an arbitrary payload to json.RawMessage
// for the envelope. Returns nil on marshal error (the envelope
// still ships, just without payload — Topic alone is informative).
func payloadToRaw(p any) json.RawMessage {
	if p == nil {
		return nil
	}
	if raw, ok := p.(json.RawMessage); ok {
		return raw
	}
	data, err := json.Marshal(p)
	if err != nil {
		return nil
	}
	return data
}
