// SPDX-License-Identifier: Apache-2.0

package render

import "testing"

func TestMediaTypeForName(t *testing.T) {
	engines := []EngineDecl{
		{Files: []EngineFile{{Suffix: ".landscape.json", MediaType: "application/vnd.leanix.landscape+json"}}},
		{Files: []EngineFile{{Suffix: ".json", MediaType: "application/x-other"}}},
	}
	cases := map[string]string{
		"C:/x/Portfolio.Landscape.JSON": "application/vnd.leanix.landscape+json", // longest suffix, any case
		"data.json":                     "application/x-other",
		"notes.md":                      "",
	}
	for name, want := range cases {
		if got := MediaTypeForName(engines, name); got != want {
			t.Errorf("MediaTypeForName(%q) = %q, want %q", name, got, want)
		}
	}
}
