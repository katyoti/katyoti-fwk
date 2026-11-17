// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
)

// An archive is a record of its own: the object ended, and the note says
// where its file went. Without its case the journal would drop the event.
func TestAnArchivedObjectIsRecorded(t *testing.T) {
	p, _ := json.Marshal(cwmapi.ObjectArchivedEventPayload{
		EventMeta: cwmapi.EventMeta{EventID: "ev-1", Revision: "r1"},
		ObjectID:  "m.RQ-001", Module: "m", Type: "request", Parent: "m",
		Path: "modules/m/.ktw/_history/2026-10/RQ-001.cwm.yaml",
	})
	rec, ok := recordOf(cwmapi.EventEnvelope{Topic: cwmapi.ObjectArchivedSubject("t", "u", "w", cwmapi.SubjectToken("m.RQ-001")), Payload: p})
	if !ok || rec.Action != api.ActionArchived {
		t.Fatalf("record = %+v (%v) — want an archived record", rec, ok)
	}
	if rec.ID != "m.RQ-001" || rec.Module != "m" || rec.Type != "request" || rec.Parent != "m" ||
		rec.Note != "modules/m/.ktw/_history/2026-10/RQ-001.cwm.yaml" || rec.EventID != "ev-1" || rec.Revision != "r1" {
		t.Errorf("record = %+v", rec)
	}
}
