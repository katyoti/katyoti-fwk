// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// A catalog written before requires carried structured needs listed bare
// names: it still loads, each name a need of that name.
func TestRequires_OldCatalogStillLoads(t *testing.T) {
	old := []byte(`apiVersion: 1
marketplace: {name: old}
items:
  - name: ktw-x
    kind: sidecar
    versions:
      - version: 1.0.0
        requires:
          formats: {skill: "1.0"}
          tools: [pandoc]
          apiKeys: [OPENAI_API_KEY]
          auth: [google]
`)
	var cat Catalog
	if err := config.DecodeBytes(old, marshal.YAML, &cat); err != nil {
		t.Fatalf("an old catalog no longer loads: %v", err)
	}
	r := cat.Items[0].Versions[0].Requires
	if len(r.Tools) != 1 || r.Tools[0].Name != "pandoc" ||
		len(r.APIKeys) != 1 || r.APIKeys[0].Name != "OPENAI_API_KEY" ||
		len(r.Auth) != 1 || r.Auth[0].Name != "google" || r.Formats["skill"] != "1.0" {
		t.Fatalf("old names lost: %+v", r)
	}

	// Over the wire too: an older hub answers bare names.
	var j Requires
	if err := json.Unmarshal([]byte(`{"tools":["pandoc"],"apiKeys":["K"],"auth":["google"]}`), &j); err != nil {
		t.Fatalf("old JSON no longer decodes: %v", err)
	}
	if len(j.Tools) != 1 || j.Tools[0].Name != "pandoc" || j.APIKeys[0].Name != "K" || j.Auth[0].Name != "google" {
		t.Fatalf("old JSON names lost: %+v", j)
	}
	// And the structured form decodes as itself.
	if err := json.Unmarshal([]byte(`{"tools":[{"name":"ffmpeg","required":true,"version":"6"}]}`), &j); err != nil {
		t.Fatal(err)
	}
	if j.Tools[0].Name != "ffmpeg" || !j.Tools[0].Required || j.Tools[0].Version != "6" {
		t.Fatalf("structured JSON = %+v", j.Tools)
	}
}
