// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"encoding/json"
	"testing"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestDecodeUseCase_BodyRoundTrip pins the bug surfaced by the
// ModuleAudit smoke 2026-05-19: UC items with a non-empty DoD body
// were coming back from the projection loader with Body == "",
// causing the MissingDoDBody lint detector to false-positive.
//
// Two root causes were fixed:
//
//  1. The decoder's UCID json-tag was `id` (legacy YAML naming) but
//     methods_uc.go.ucBody() writes `uc_id` per the rename to avoid
//     collision with the item-substrate UUID. We now accept both,
//     preferring `uc_id`.
//  2. The decoder had no Body field at all — the DoD markdown was
//     silently dropped on every load.
//
// Each sub-test pins one shape that should survive the round-trip.
func TestDecodeUseCase_BodyRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body map[string]any
		want gcapi.UseCase
	}{
		{
			name: "canonical (uc_id + body + severity + status)",
			body: map[string]any{
				"uc_id":        "UC-001",
				"severity":     "core",
				"status_cache": "covered",
				"body":         "## DoD\n- L1 test must pass",
			},
			want: gcapi.UseCase{
				UCID:        "UC-001",
				Severity:    gcapi.UCSeverityCore,
				StatusCache: gcapi.UCStatusCovered,
				Body:        "## DoD\n- L1 test must pass",
			},
		},
		{
			name: "legacy id key still parses",
			body: map[string]any{
				"id":           "UC-042",
				"severity":     "security",
				"status_cache": "validated",
				"body":         "legacy item",
			},
			want: gcapi.UseCase{
				UCID:        "UC-042",
				Severity:    gcapi.UCSeveritySecurity,
				StatusCache: gcapi.UCStatusValidated,
				Body:        "legacy item",
			},
		},
		{
			name: "uc_id wins when both keys present",
			body: map[string]any{
				"uc_id":        "UC-NEW",
				"id":           "UC-OLD",
				"severity":     "qol",
				"status_cache": "no-test",
			},
			want: gcapi.UseCase{
				UCID:        "UC-NEW",
				Severity:    gcapi.UCSeverityQoL,
				StatusCache: gcapi.UCStatusNoTest,
			},
		},
		{
			name: "fail-deferred carries FailDeferredBy",
			body: map[string]any{
				"uc_id":            "UC-003",
				"severity":         "qol",
				"status_cache":     "fail-deferred",
				"fail_deferred_by": "OD-001",
				"body":             "deferred until lib decision",
			},
			want: gcapi.UseCase{
				UCID:           "UC-003",
				Severity:       gcapi.UCSeverityQoL,
				StatusCache:    gcapi.UCStatusFailDeferred,
				FailDeferredBy: "OD-001",
				Body:           "deferred until lib decision",
			},
		},
		{
			name: "empty body field survives as empty UC.Body",
			body: map[string]any{
				"uc_id":        "UC-004",
				"severity":     "core",
				"status_cache": "no-test",
			},
			want: gcapi.UseCase{
				UCID:        "UC-004",
				Severity:    gcapi.UCSeverityCore,
				StatusCache: gcapi.UCStatusNoTest,
				Body:        "",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bytes, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("marshal fixture: %v", err)
			}
			// The fixture is the old flat record; the object holds it where each
			// value lives now: the UC id is the authored id, the DoD prose the
			// body, the severity the checkable capability's, the rest the fields.
			it := cwmapi.Object{ID: "item-uuid", Type: "use-case"}
			it.Parent = "node-uuid"
			it.Name = "test UC"
			it.SetFieldsJSON(bytes)
			for _, k := range []string{"id", "uc_id"} {
				if id, ok := it.Fields[k].(string); ok {
					it.ID = "m." + id
				}
				delete(it.Fields, k)
			}
			if b, ok := it.Fields["body"].(string); ok {
				it.Body = b
				delete(it.Fields, "body")
			}
			if sev, ok := it.Fields["severity"]; ok {
				it.Capabilities = map[string]map[string]any{"checkable": {"severity": sev}}
				delete(it.Fields, "severity")
			}
			// The deferring decision is a `deferred-by` edge, never a field.
			if od, ok := it.Fields["fail_deferred_by"].(string); ok {
				it.SetEdgeTo(gcapi.EdgeDeferredBy, "m."+od)
				delete(it.Fields, "fail_deferred_by")
			}
			got, err := decodeUseCase(it)
			if err != nil {
				t.Fatalf("decodeUseCase: %v", err)
			}
			if got.UCID != tc.want.UCID {
				t.Errorf("UCID = %q want %q", got.UCID, tc.want.UCID)
			}
			if got.Body != tc.want.Body {
				t.Errorf("Body = %q want %q", got.Body, tc.want.Body)
			}
			if got.Severity != tc.want.Severity {
				t.Errorf("Severity = %q want %q", got.Severity, tc.want.Severity)
			}
			if got.StatusCache != tc.want.StatusCache {
				t.Errorf("StatusCache = %q want %q", got.StatusCache, tc.want.StatusCache)
			}
			if got.FailDeferredBy != tc.want.FailDeferredBy {
				t.Errorf("FailDeferredBy = %q want %q", got.FailDeferredBy, tc.want.FailDeferredBy)
			}
		})
	}
}

// TestDecodeUseCase_AuthoredIDWins pins the identity collapse: once an object
// carries a module-qualified id, THAT is the use-case's human id and the body's
// own uc_id no longer governs. Without this, a stale body field could quietly
// disagree with the filename.
func TestDecodeUseCase_AuthoredIDWins(t *testing.T) {
	body, err := json.Marshal(map[string]any{"uc_id": "STALE-999", "severity": "core"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	o := cwmapi.Object{ID: "compose.CMP-010", Type: "use-case"}
	o.Parent = "compose"
	o.Name = "routes a skill request"
	o.SetFieldsJSON(body)
	o.Capabilities = map[string]map[string]any{"checkable": {"severity": "core"}}

	got, err := decodeUseCase(o)
	if err != nil {
		t.Fatalf("decodeUseCase: %v", err)
	}
	if got.UCID != "CMP-010" {
		t.Errorf("UCID = %q, want CMP-010 — the authored id must win over a stale body uc_id", got.UCID)
	}
	if got.NodeID != "compose" {
		t.Errorf("NodeID = %q, want compose (container.parent)", got.NodeID)
	}
	if got.Severity != gcapi.UCSeverityCore {
		t.Errorf("Severity = %q, want core", got.Severity)
	}
}
