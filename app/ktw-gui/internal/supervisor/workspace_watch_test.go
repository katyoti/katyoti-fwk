// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"encoding/json"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	sourcesapi "github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// The GUI forwards WHICH object an event is about, read from the payload CWM
// publishes — built here with CWM's own payload types and subject builders, so
// a renamed key or token on either side breaks this test.
func TestWorkspaceEventOfForwardsTheObjectOfEachCWMEvent(t *testing.T) {
	enc := func(v any) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	tok := cwmapi.SubjectToken("billing.uc-1")
	cases := map[string]struct {
		subject string
		payload []byte
		want    WorkspaceEvent
	}{
		"an update": {
			cwmapi.ObjectUpdatedSubject("t", "u", "w", tok),
			enc(cwmapi.ObjectEventPayload{ObjectID: "billing.uc-1", Module: "billing", Type: "use-case", Parent: "billing", Action: "updated"}),
			WorkspaceEvent{Object: "billing.uc-1", Module: "billing", Type: "use-case", Parent: "billing", Action: "updated"},
		},
		"a transition": {
			cwmapi.ObjectTransitionedSubject("t", "u", "w", tok),
			enc(cwmapi.ObjectTransitionedEventPayload{ObjectID: "billing.uc-1", Module: "billing", Type: "use-case", From: "a", To: "b"}),
			WorkspaceEvent{Object: "billing.uc-1", Module: "billing", Type: "use-case", Action: "transitioned"},
		},
		"a move says where the object was": {
			cwmapi.ObjectMovedSubject("t", "u", "w", tok),
			enc(cwmapi.ObjectMovedEventPayload{ObjectID: "billing.uc-1", Module: "billing", Parent: "billing.f-1", FromID: "pay.uc-1", FromModule: "pay", FromParent: "pay", Type: "use-case"}),
			WorkspaceEvent{Object: "billing.uc-1", Module: "billing", Type: "use-case", Parent: "billing.f-1", Action: "moved", FromModule: "pay", FromParent: "pay"},
		},
		"a raised event carries its type and payload": {
			cwmapi.ObjectEventRaisedSubject("t", "u", "w", tok),
			enc(cwmapi.ObjectRaisedEventPayload{ObjectID: "billing.uc-1", Module: "billing", Type: "information-source", Event: sourcesapi.EventSourceSync,
				Summary: string(enc(sourcesapi.SourceSyncProgress{Source: "billing.uc-1", Phase: sourcesapi.SyncItems, Read: 3, Total: 7}))}),
			WorkspaceEvent{Object: "billing.uc-1", Module: "billing", Type: "information-source", Action: "event", Event: "source.sync",
				Payload: `{"source":"billing.uc-1","phase":"items","read":3,"total":7}`},
		},
		"an undecodable payload keeps the action": {
			cwmapi.ObjectDeletedSubject("t", "u", "w", tok),
			[]byte("not json"),
			WorkspaceEvent{Action: "deleted"},
		},
	}
	for name, c := range cases {
		got := WorkspaceEventOf("u", c.subject, c.payload)
		c.want.Universe, c.want.Subject = "u", c.subject
		if got != c.want {
			t.Errorf("%s:\n got %+v\nwant %+v", name, got, c.want)
		}
	}
}
