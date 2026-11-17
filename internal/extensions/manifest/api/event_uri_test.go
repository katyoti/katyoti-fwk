// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"errors"
	"reflect"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

func TestParseEventURI_Valid(t *testing.T) {
	cases := []struct {
		s    string
		want mbapi.EventURI
	}{
		{
			"cwm:!workspace/object/abc-123/created",
			mbapi.EventURI{Provider: "cwm", Scope: "workspace", Path: []string{"object", "abc-123", "created"}},
		},
		{
			"cwm:!workspace/object/*/created",
			mbapi.EventURI{Provider: "cwm", Scope: "workspace", Path: []string{"object", "*", "created"}},
		},
		{
			"ktw-agents:!session/task/run-xyz/started",
			mbapi.EventURI{Provider: "ktw-agents", Scope: "session", Path: []string{"task", "run-xyz", "started"}},
		},
		{
			"cwm:!universe/connstate-changed",
			mbapi.EventURI{Provider: "cwm", Scope: "universe", Path: []string{"connstate-changed"}},
		},
		{
			"cwm:!tenant/billing-changed",
			mbapi.EventURI{Provider: "cwm", Scope: "tenant", Path: []string{"billing-changed"}},
		},
		{
			"cwm:!room/abc/event/joined",
			mbapi.EventURI{Provider: "cwm", Scope: "room", Path: []string{"abc", "event", "joined"}},
		},
		{
			":!workspace/object/*/updated",
			mbapi.EventURI{Provider: "", Scope: "workspace", Path: []string{"object", "*", "updated"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.s, func(t *testing.T) {
			got, err := mbapi.ParseEventURI(tc.s)
			if err != nil {
				t.Fatalf("ParseEventURI(%q) error: %v", tc.s, err)
			}
			if got.Provider != tc.want.Provider || got.Scope != tc.want.Scope || !reflect.DeepEqual(got.Path, tc.want.Path) {
				t.Errorf("ParseEventURI(%q) = %+v, want %+v", tc.s, got, tc.want)
			}
			if back := got.String(); back != tc.s {
				t.Errorf("String() round-trip drift: %q → %q", tc.s, back)
			}
		})
	}
}

func TestParseEventURI_Invalid(t *testing.T) {
	cases := []struct {
		name string
		s    string
	}{
		{"missing colon", "cwm!workspace/object/abc/created"},
		{"tool URI not event", "cwm:workspace/node/abc/created"},
		{"resource URI not event", "cwm:/workspace/node/abc/created"},
		{"unknown scope", "cwm:!cosmic/node/abc/created"},
		{"missing path", "cwm:!workspace/"},
		{"missing path slash", "cwm:!workspace"},
		{"empty middle segment", "cwm:!workspace/object//created"},
		{"uppercase in scope", "cwm:!Workspace/node/abc/created"},
		{"underscore in segment", "cwm:!workspace/object/abc_123/created"},
		{"empty string", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mbapi.ParseEventURI(tc.s)
			if err == nil {
				t.Fatalf("ParseEventURI(%q): expected error, got nil", tc.s)
			}
			if !errors.Is(err, mbapi.ErrInvalidEventURI) {
				t.Errorf("ParseEventURI(%q): expected ErrInvalidEventURI, got %v", tc.s, err)
			}
		})
	}
}

func TestEventURI_Subject(t *testing.T) {
	cases := []struct {
		name        string
		uri         string
		tenantID    string
		universeID  string
		scopeID     string
		wantSubject string
	}{
		{
			"workspace scope concrete",
			"cwm:!workspace/object/abc/created",
			"local", "default", "ws-1",
			"ktw.local.universe.default.workspace.ws-1.object.abc.created",
		},
		{
			"workspace scope wildcard",
			"cwm:!workspace/object/*/created",
			"local", "default", "ws-1",
			"ktw.local.universe.default.workspace.ws-1.object.*.created",
		},
		{
			"session scope task lifecycle",
			"ktw-agents:!session/task/run-xyz/started",
			"local", "default", "sess-1",
			"ktw.local.universe.default.session.sess-1.task.run-xyz.started",
		},
		{
			"universe scope event",
			"cwm:!universe/connstate-changed",
			"local", "default", "",
			"ktw.local.universe.default.connstate-changed",
		},
		{
			"tenant scope event",
			"cwm:!tenant/billing-changed",
			"local", "", "",
			"ktw.local.billing-changed",
		},
		{
			"room scope event",
			"cwm:!room/abc/joined",
			"local", "default", "room-7",
			"ktw.local.universe.default.room.room-7.abc.joined",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := mbapi.ParseEventURI(tc.uri)
			if err != nil {
				t.Fatalf("ParseEventURI(%q): %v", tc.uri, err)
			}
			got := u.Subject(tc.tenantID, tc.universeID, tc.scopeID)
			if got != tc.wantSubject {
				t.Errorf("Subject:\n  got  %q\n  want %q", got, tc.wantSubject)
			}
		})
	}
}

func TestIsEventURI(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"cwm:!workspace/object/abc/created", true},
		{":!workspace/object/abc/created", true},
		{"cwm:/skill/concern-report", false},
		{"cwm:tasks/dossier/refine-scope", false},
		{":/skill/foo", false},
		{":weather-service/get", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := mbapi.IsEventURI(tc.s); got != tc.want {
			t.Errorf("IsEventURI(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

// TestParseURI_AutoDetect_EventArm — the unified parser's third arm
// dispatches `!` to EventURI.
func TestParseURI_AutoDetect_EventArm(t *testing.T) {
	cases := []struct {
		s        string
		wantKind mbapi.URIKind
	}{
		{"cwm:!workspace/object/abc/created", mbapi.KindEvent},
		{"cwm:!session/task/run-xyz/started", mbapi.KindEvent},
		{"cwm:/skill/concern-report", mbapi.KindResource},
		{"ktw-acme:tasks/dossier/refine-scope", mbapi.KindTool},
	}
	for _, tc := range cases {
		t.Run(tc.s, func(t *testing.T) {
			got, err := mbapi.ParseURI(tc.s)
			if err != nil {
				t.Fatalf("ParseURI(%q): %v", tc.s, err)
			}
			if got.Kind != tc.wantKind {
				t.Errorf("ParseURI(%q).Kind = %v, want %v", tc.s, got.Kind, tc.wantKind)
			}
			if back := got.String(); back != tc.s {
				t.Errorf("String() drift: %q → %q", tc.s, back)
			}
			switch tc.wantKind {
			case mbapi.KindEvent:
				if !got.IsEvent() || got.IsResource() || got.IsTool() {
					t.Errorf("IsEvent classifier mismatch: %+v", got)
				}
			case mbapi.KindResource:
				if !got.IsResource() || got.IsEvent() || got.IsTool() {
					t.Errorf("IsResource classifier mismatch: %+v", got)
				}
			case mbapi.KindTool:
				if !got.IsTool() || got.IsEvent() || got.IsResource() {
					t.Errorf("IsTool classifier mismatch: %+v", got)
				}
			}
		})
	}
}
