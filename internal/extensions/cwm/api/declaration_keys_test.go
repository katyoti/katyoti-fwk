// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// strictKinds is one known-good declaration per kind, the same declaration with
// one misspelt key, and that key — what the refusal must name.
var strictKinds = []struct {
	kind, contribType string
	decode            func() any
	good, bad, key    string
}{
	{"capability", "capability", func() any { return &cwmapi.Capability{} },
		`{"name":"c","label":"C"}`, `{"name":"c","lable":"C"}`, "lable"},
	{"edge type", "edge-type", func() any { return &cwmapi.EdgeTypeRegistration{} },
		`{"name":"e","from":"any","to":"any","cardinality":"N:1"}`, `{"name":"e","from":"any","to":"any","cardinalty":"N:1"}`, "cardinalty"},
	{"aspect", "aspect", func() any { return &cwmapi.Aspect{} },
		`{"name":"a","capabilities":["c"]}`, `{"name":"a","capabilites":["c"]}`, "capabilites"},
	{"workflow", "workflow", func() any { return &cwmapi.Workflow{} },
		`{"name":"w","initial":"a","states":[{"name":"a"}]}`, `{"name":"w","intial":"a","states":[{"name":"a"}]}`, "intial"},
	{"policy", "policy", func() any { return &cwmapi.Policy{} },
		`{"name":"p","applies":{"types":["*"]}}`, `{"name":"p","aplies":{"types":["*"]}}`, "aplies"},
	{"hook", "hook", func() any { return &cwmapi.Hook{} },
		`{"name":"h","type":"lint","uri":"x:y/z"}`, `{"name":"h","type":"lint","uri":"x:y/z","wehn":"true"}`, "wehn"},
	{"hook type", "hook-type", func() any { return &cwmapi.HookType{} },
		`{"name":"review","timing":"on-demand","answer":"findings"}`, `{"name":"review","timming":"on-demand","answer":"findings"}`, "timming"},
}

// Every declaration kind refuses a key no field takes, naming it — through
// JSON (the catalog) and YAML (the pack) alike; a known-good one loads.
func TestEveryDeclarationKindRefusesAnUnknownKey(t *testing.T) {
	for _, k := range strictKinds {
		t.Run(k.kind, func(t *testing.T) {
			if err := json.Unmarshal([]byte(k.good), k.decode()); err != nil {
				t.Errorf("json: a known-good %s is refused: %v", k.kind, err)
			}
			if err := yaml.Unmarshal([]byte(k.good), k.decode()); err != nil {
				t.Errorf("yaml: a known-good %s is refused: %v", k.kind, err)
			}
			for codec, decode := range map[string]func([]byte, any) error{"json": json.Unmarshal, "yaml": yaml.Unmarshal} {
				err := decode([]byte(k.bad), k.decode())
				if err == nil || !strings.Contains(err.Error(), k.key) || !strings.Contains(err.Error(), k.kind) {
					t.Errorf("%s: a %s with %q decoded with err=%v, want a refusal naming the key", codec, k.kind, k.key, err)
				}
			}
		})
	}
}

// The catalog reader skips a declaration carrying an unknown key and still
// reads its valid neighbour of the same kind.
func TestTheCatalogSkipsADeclarationWithAnUnknownKey(t *testing.T) {
	var contribs []contribapi.Contribution
	for _, k := range strictKinds {
		contribs = append(contribs,
			contribapi.Contribution{Provider: "ktw-fake", Type: k.contribType, ID: "good", Layers: contribapi.InlineData(json.RawMessage(k.good))},
			contribapi.Contribution{Provider: "ktw-fake", Type: k.contribType, ID: "bad", Layers: contribapi.InlineData(json.RawMessage(strings.Replace(k.bad, `"name":"`, `"name":"bad-`, 1)))},
		)
	}
	set := cwmapi.DeclarationsFromCatalog(context.Background(), manifestlocal.NewMemoryCatalog(
		[]*mbapi.Manifest{{Provider: "ktw-fake", Contributions: contribs}}, []string{"ktw-fake"}))
	for _, row := range []struct {
		kind string
		n    int
	}{
		{"capability", len(set.Capabilities)}, {"edge-type", len(set.Edges)}, {"aspect", len(set.Aspects)},
		{"workflow", len(set.Workflows)}, {"policy", len(set.Policies)}, {"hook", len(set.Hooks)}, {"hook-type", len(set.HookTypes)},
	} {
		if row.n != 1 {
			t.Errorf("%s: read %d declarations, want 1 (the misspelt one is skipped)", row.kind, row.n)
		}
	}
}

// The keys removed from edge types are refused like any other unknown key.
func TestARemovedEdgeTypeKeyIsRefused(t *testing.T) {
	for _, key := range []string{"writer: human", "freshness: live", "storage: field"} {
		var et cwmapi.EdgeTypeRegistration
		err := yaml.Unmarshal([]byte("name: e\nfrom: any\nto: any\n"+key+"\n"), &et)
		name := strings.SplitN(key, ":", 2)[0]
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: decoded with err=%v, want a refusal naming it", name, err)
		}
	}
}
