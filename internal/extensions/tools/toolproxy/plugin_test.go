// SPDX-License-Identifier: Apache-2.0

package toolproxy

import (
	"encoding/json"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

func sampleManifest() *mbapi.Manifest {
	return &mbapi.Manifest{
		SlotTypes: []mbapi.SlotType{{
			Type: "WeatherService", // PascalCase in descriptor; matched via kebab
			Methods: []mbapi.SlotMethod{
				{Name: "GetForecast", Description: "7-day forecast", InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)},
				{Name: "GetCurrent", Description: "current weather"},
				{Name: "InternalReindex", Tags: []string{"internal"}},
			},
		}},
	}
}

func TestFindMethod_KebabMatch(t *testing.T) {
	man := sampleManifest()
	// URI is kebab (weather-service / get-forecast); descriptor is PascalCase.
	sm := findMethod(man, "weather-service", "get-forecast")
	if sm == nil {
		t.Fatal("expected to find get-forecast via kebab match")
	}
	if sm.Description != "7-day forecast" || len(methodSchema(sm)) == 0 {
		t.Errorf("wrong method resolved: desc=%q schema=%s", sm.Description, methodSchema(sm))
	}
	if findMethod(man, "weather-service", "nope") != nil {
		t.Error("unknown method should resolve to nil")
	}
	if findMethod(nil, "weather-service", "get-forecast") != nil {
		t.Error("nil manifest should resolve to nil")
	}
}

func TestMethodAdvertised(t *testing.T) {
	man := sampleManifest()
	if !methodAdvertised(findMethod(man, "weather-service", "get-forecast")) {
		t.Error("get-forecast should be advertised (no internal tag)")
	}
	if methodAdvertised(findMethod(man, "weather-service", "internal-reindex")) {
		t.Error("internal-tagged method must NOT be advertised (excluded from wildcards)")
	}
	// A person-only method (meta.MethodDef.NoLLM → tag mcp=false) is no tool
	// either: `gates/*` must not hand an agent the gate confirmation.
	if methodAdvertised(&mbapi.SlotMethod{Name: "confirm", Tags: []string{"mcp=false"}}) {
		t.Error("an mcp=false method must NOT be advertised to an agent")
	}
}

func TestToolName(t *testing.T) {
	cases := map[string]string{
		"get-forecast":         "get-forecast",
		"dossier/refine-scope": "refine-scope", // multi-segment → last segment
	}
	for method, want := range cases {
		got := toolName(mbapi.ToolURI{Provider: "p", SlotType: "s", Method: method})
		if got != want {
			t.Errorf("toolName(%q) = %q, want %q", method, got, want)
		}
	}
}

func TestIsGlobAndTrim(t *testing.T) {
	if !isGlob("*") || !isGlob("get-*") || isGlob("get-forecast") {
		t.Error("isGlob misclassified a selector")
	}
	got := trimmedUnique([]string{" a ", "a", "", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("trimmedUnique = %v, want [a b]", got)
	}
}
