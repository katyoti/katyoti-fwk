// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"errors"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

func TestParseToolURI_Valid(t *testing.T) {
	cases := []struct {
		s    string
		want mbapi.ToolURI
	}{
		{
			"ktw-weather:weather-service/get-forecast",
			mbapi.ToolURI{Provider: "ktw-weather", SlotType: "weather-service", Instance: "default", Method: "get-forecast"},
		},
		{
			"ktw-example:leanix-service(prod)/search",
			mbapi.ToolURI{Provider: "ktw-example", SlotType: "leanix-service", Instance: "prod", Method: "search"},
		},
		{
			":weather-service/get-current",
			mbapi.ToolURI{Provider: "", SlotType: "weather-service", Instance: "default", Method: "get-current"},
		},
		{
			"ktw-docs:script-service/elk-layout",
			mbapi.ToolURI{Provider: "ktw-docs", SlotType: "script-service", Instance: "default", Method: "elk-layout"},
		},
		{
			"ktw-mcp-bridge:external-mcp(notion)/append-block",
			mbapi.ToolURI{Provider: "ktw-mcp-bridge", SlotType: "external-mcp", Instance: "notion", Method: "append-block"},
		},
		// Multi-segment methods (cwm-v2-providers US-15, 2026-06-08).
		{
			"ktw-acme:tasks/dossier/refine-scope",
			mbapi.ToolURI{Provider: "ktw-acme", SlotType: "tasks", Instance: "default", Method: "dossier/refine-scope"},
		},
		{
			"ktw-agents:tasks/inbox/seed-default-items",
			mbapi.ToolURI{Provider: "ktw-agents", SlotType: "tasks", Instance: "default", Method: "inbox/seed-default-items"},
		},
		{
			"ktw-agents:tasks/deeply/nested/four/segments",
			mbapi.ToolURI{Provider: "ktw-agents", SlotType: "tasks", Instance: "default", Method: "deeply/nested/four/segments"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.s, func(t *testing.T) {
			got, err := mbapi.ParseToolURI(tc.s)
			if err != nil {
				t.Fatalf("ParseToolURI(%q) error: %v", tc.s, err)
			}
			if got != tc.want {
				t.Errorf("ParseToolURI(%q) = %+v, want %+v", tc.s, got, tc.want)
			}
			// Round-trip via String.
			if back := got.String(); back != tc.s {
				t.Errorf("String() round-trip drift: parsed %q → struct → String() = %q", tc.s, back)
			}
		})
	}
}

func TestParseToolURI_Invalid(t *testing.T) {
	cases := []struct {
		name string
		s    string
	}{
		{"missing colon", "ktw-weather/weather-service/get-forecast"},
		{"resource URI form (slash after colon)", "ktw-weather:/weather-service/get-forecast"},
		{"missing method slash", "ktw-weather:weather-service"},
		{"empty slot", "ktw-weather:/get-forecast"}, // this is a resource URI; tool parser rejects
		{"empty method", "ktw-weather:weather-service/"},
		{"uppercase slot", "ktw-weather:Weather-Service/get-forecast"},
		{"unbalanced parens", "ktw-weather:weather-service(prod/get-forecast"},
		{"underscore in method", "ktw-weather:weather-service/get_forecast"},
		{"dot in slot", "ktw-weather:weather.service/get-forecast"},
		{"empty string", ""},
		// Multi-segment rejection cases (US-15).
		{"empty middle segment", "ktw-agents:tasks/dossier//refine-scope"},
		{"trailing slash in method", "ktw-agents:tasks/dossier/refine-scope/"},
		{"uppercase in middle segment", "ktw-agents:tasks/Dossier/refine-scope"},
		{"underscore in segment", "ktw-agents:tasks/dossier/refine_scope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mbapi.ParseToolURI(tc.s)
			if err == nil {
				t.Fatalf("ParseToolURI(%q): expected error, got nil", tc.s)
			}
			if !errors.Is(err, mbapi.ErrInvalidToolURI) {
				t.Errorf("ParseToolURI(%q): expected ErrInvalidToolURI, got %v", tc.s, err)
			}
		})
	}
}

// TestSubject_RoundTripWithEngineBuilder pins ToolURI.Subject against
// subjects.SlotSubject. If they drift this test fails loudly. Per
// `feedback_pin_parser_to_builder` memory.
func TestSubject_RoundTripWithEngineBuilder(t *testing.T) {
	cases := []mbapi.ToolURI{
		{Provider: "ktw-weather", SlotType: "weather-service", Instance: "default", Method: "get-forecast"},
		{Provider: "ktw-example", SlotType: "leanix-service", Instance: "prod", Method: "search"},
		{Provider: "ktw-docs", SlotType: "script-service", Instance: "default", Method: "elk-layout"},
		// Multi-segment methods round-trip the subject builder
		// (US-15): `/` in the URI becomes `.` in the subject.
		{Provider: "ktw-acme", SlotType: "tasks", Instance: "default", Method: "dossier/refine-scope"},
		{Provider: "ktw-agents", SlotType: "tasks", Instance: "default", Method: "deeply/nested/four/segments"},
	}
	tenantID := "local"
	universeID := "acme"
	for _, u := range cases {
		t.Run(u.String(), func(t *testing.T) {
			fromURI := u.Subject(tenantID, universeID, u.Provider)
			fromEngine := subjects.SlotSubject(tenantID, universeID, u.Provider, u.SlotType, u.Instance, u.Method)
			if fromURI != fromEngine {
				t.Errorf("Subject drift:\n  ToolURI.Subject() = %q\n  subjects.SlotSubject = %q", fromURI, fromEngine)
			}
			// Round-trip via ToolURIFromSubject.
			back, err := mbapi.ToolURIFromSubject(fromURI)
			if err != nil {
				t.Fatalf("ToolURIFromSubject(%q) error: %v", fromURI, err)
			}
			if back != u {
				t.Errorf("ToolURIFromSubject round-trip drift: got %+v, want %+v", back, u)
			}
		})
	}
}

// TestParseURI_AutoDetect confirms the unified parser chooses the
// right shape based on the byte after ':'.
func TestParseURI_AutoDetect(t *testing.T) {
	cases := []struct {
		s        string
		wantKind mbapi.URIKind
	}{
		{"ktw-docs:/skill/concern-report", mbapi.KindResource},
		{"ktw-docs:/data-type/skill-body", mbapi.KindResource},
		{":/skill/weather-usage", mbapi.KindResource},
		{"ktw-example:leanix-service/search", mbapi.KindTool},
		{"ktw-weather:weather-service(prod)/get-forecast", mbapi.KindTool},
		{":weather-service/get-current", mbapi.KindTool},
	}
	for _, tc := range cases {
		t.Run(tc.s, func(t *testing.T) {
			got, err := mbapi.ParseURI(tc.s)
			if err != nil {
				t.Fatalf("ParseURI(%q) error: %v", tc.s, err)
			}
			if got.Kind != tc.wantKind {
				t.Errorf("ParseURI(%q).Kind = %v, want %v", tc.s, got.Kind, tc.wantKind)
			}
			if back := got.String(); back != tc.s {
				t.Errorf("String() drift: %q → %q", tc.s, back)
			}
		})
	}
}

// TestIsToolURI cheap classifier.
func TestIsToolURI(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"ktw-example:leanix-service/search", true},
		{"ktw-example:leanix-service(prod)/search", true},
		{":weather-service/get-current", true},
		{"ktw-docs:/skill/concern-report", false},
		{":/skill/foo", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := mbapi.IsToolURI(tc.s); got != tc.want {
			t.Errorf("IsToolURI(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

// TestNewToolURI_NormalizesEmptyInstance — passing "" for instance
// should be normalized to "default" so callers don't have to remember.
func TestNewToolURI_NormalizesEmptyInstance(t *testing.T) {
	u, err := mbapi.NewToolURI("ktw-weather", "weather-service", "", "get-current")
	if err != nil {
		t.Fatalf("NewToolURI: %v", err)
	}
	if u.Instance != "default" {
		t.Errorf("Instance = %q, want default", u.Instance)
	}
	if u.String() != "ktw-weather:weather-service/get-current" {
		t.Errorf("String() with default instance should omit parens, got: %s", u.String())
	}
}

// TestToolURI_TokenTooLong — token length cap is 63 per the kebab rule.
func TestToolURI_TokenTooLong(t *testing.T) {
	long := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 66 'a's
	_, err := mbapi.NewToolURI("ktw-weather", long, "default", "go")
	if err == nil {
		t.Fatal("expected error for >63-char slot")
	}
}
