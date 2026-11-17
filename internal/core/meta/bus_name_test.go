// SPDX-License-Identifier: Apache-2.0

package meta

import "testing"

// TestDefaultBusName covers the kebab-conversion edge cases the
// named-slot-addressing scheme depends on. Acronym runs are the
// interesting cases: a naive "insert '-' before every uppercase" loses
// the acronym group boundary (LLMProfiles → l-l-m-profiles instead of
// llm-profiles). The implementation has to look at runs of consecutive
// uppercase letters and only split before the lowercase tail.
func TestDefaultBusName(t *testing.T) {
	cases := map[string]string{
		// Simple PascalCase
		"StartTask":    "start-task",
		"List":         "list",
		"Get":          "get",
		"GetReference": "get-reference",

		// Single-acronym prefix → the boundary before the final lowercase
		// run gets the separator, the rest of the acronym stays glued.
		"LLMProfiles": "llm-profiles",
		"MCPBridge":   "mcp-bridge",
		"HTTPClient":  "http-client",
		"URLBuilder":  "url-builder",
		"REST":        "rest",

		// Back-to-back acronym runs are genuinely ambiguous from spelling
		// alone — "RESTAPIClient" could mean (REST, API, Client) or
		// (RESTA, PI, Client) and the algorithm can't tell. Authors who
		// need a specific kebab form set ServiceDef.BusName explicitly
		// (see TestResolvedBusName below). The default here glues all
		// consecutive acronym letters into one run and splits only before
		// the trailing lowercase word.
		"RESTAPIClient": "restapi-client",

		// Already kebab — idempotent.
		"resource-service": "resource-service",
		"llm-profiles":     "llm-profiles",
		"docs":             "docs",

		// Digits stay glued to their adjacent run.
		"OAuth2Provider": "o-auth2-provider",

		// Single character
		"A": "a",
		"a": "a",
		"":  "",
	}
	for in, want := range cases {
		got := DefaultBusName(in)
		if got != want {
			t.Errorf("DefaultBusName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestResolvedBusName confirms the explicit-value override path: an
// author who declares ServiceDef{Name: "ResourceService", BusName:
// "resources"} gets "resources" on the wire (canonical short form),
// not the mechanical "resource-service".
func TestResolvedBusName(t *testing.T) {
	cases := []struct {
		def  ServiceDef
		want string
	}{
		{ServiceDef{Name: "StartTask"}, "start-task"},                              // default
		{ServiceDef{Name: "ResourceService"}, "resource-service"},                  // default
		{ServiceDef{Name: "ResourceService", BusName: "resources"}, "resources"},   // explicit override
		{ServiceDef{Name: "LLMProfiles"}, "llm-profiles"},                          // default acronym
		{ServiceDef{Name: "LLMProfiles", BusName: "llm-profiles"}, "llm-profiles"}, // explicit (matches default)
		{ServiceDef{Name: "LLMProfiles", BusName: "models"}, "models"},             // explicit shorter form
	}
	for _, c := range cases {
		got := c.def.ResolvedBusName()
		if got != c.want {
			t.Errorf("ResolvedBusName for %+v = %q, want %q", c.def, got, c.want)
		}
	}
}
