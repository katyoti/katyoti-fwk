// SPDX-License-Identifier: Apache-2.0

package contribution

import (
	"context"
	"encoding/json"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"testing"
)

// TestLoadSystemContributions_Snapshot asserts the set of URIs emitted
// from the embedded YAML tree matches a stable expected list.
// Failures here mean either:
//   - a new YAML file was added and we want to bless it (update the
//     `want` slice below), OR
//   - a typo / rename silently broke the bootstrap chain.
//
// Per contribution-meta-model.md §4 the system layer is bootstrap-
// critical — every binary must carry the same set, and consumer code
// (validators, hub aggregation) keys off URI identity. A drift here
// produces opaque "data-type not found" errors at runtime; the
// snapshot test catches it at CI time.
func TestLoadSystemContributions_Snapshot(t *testing.T) {
	items, err := loadSystemContributions()
	if err != nil {
		t.Fatalf("loadSystemContributions: %v", err)
	}

	// Phase D-4 shipped 3 files to prove the chain; Phase E-4 added
	// the sidecar-info pair (contribution-type + data-type) so every
	// binary that wires `appcore.SidecarExt(...)` emits a uniform
	// identity Contribution backed by an embedded schema. cwm-v3
	// (2026-06-06) added the node-type + item-type pairs.
	//
	// 2026-06-09 — system contribution cleanup: data-type ids kebab'd
	// (Contribution→contribution, NodeType→node-type,
	// ItemTypeRegistration→item-type) and the set completed so EVERY
	// system contribution-type has BOTH a `contribution-type/<name>`
	// instance AND a `data-type/<name>` body schema. URIs are walked in
	// lexical order (WalkDir): all of contribution-type/ then all of
	// data-type/. capability-panel is one of them.
	want := []string{
		"system:/contribution-type/agent-context",
		"system:/contribution-type/aspect",
		"system:/contribution-type/capability",
		"system:/contribution-type/config-file",
		"system:/contribution-type/config-program",
		"system:/contribution-type/contribution-type",
		"system:/contribution-type/data-type",
		"system:/contribution-type/display-engine",
		"system:/contribution-type/edge-type",
		"system:/contribution-type/env",
		"system:/contribution-type/event-type",
		"system:/contribution-type/gate",
		"system:/contribution-type/gate-requirement",
		"system:/contribution-type/guide-recipe",
		"system:/contribution-type/hook",
		"system:/contribution-type/hook-type",
		"system:/contribution-type/object-panel",
		"system:/contribution-type/object-type",
		"system:/contribution-type/page-template",
		"system:/contribution-type/panel-component",
		"system:/contribution-type/panel-type",
		"system:/contribution-type/policy",
		"system:/contribution-type/reference",
		"system:/contribution-type/review-type",
		"system:/contribution-type/service",
		"system:/contribution-type/sidecar-info",
		"system:/contribution-type/universe-tag",
		"system:/contribution-type/workflow",
		"system:/data-type/agent-context",
		"system:/data-type/aspect",
		"system:/data-type/capability",
		"system:/data-type/config-file",
		"system:/data-type/config-program",
		"system:/data-type/contribution",
		"system:/data-type/contribution-type",
		"system:/data-type/data-type",
		"system:/data-type/display-engine",
		"system:/data-type/document",
		"system:/data-type/edge-type",
		"system:/data-type/env",
		"system:/data-type/event-type",
		"system:/data-type/gate",
		"system:/data-type/gate-requirement",
		"system:/data-type/guide-recipe",
		"system:/data-type/hook",
		"system:/data-type/hook-type",
		"system:/data-type/object-event",
		"system:/data-type/object-panel",
		"system:/data-type/object-transition-refused-event",
		"system:/data-type/object-transitioned-event",
		"system:/data-type/object-type",
		"system:/data-type/objects-caught-up-event",
		"system:/data-type/page-template",
		"system:/data-type/panel-component",
		"system:/data-type/panel-type",
		"system:/data-type/policy",
		"system:/data-type/reference",
		"system:/data-type/render-def",
		"system:/data-type/review-type",
		"system:/data-type/service",
		"system:/data-type/sidecar-info",
		"system:/data-type/universe-tag",
		"system:/data-type/workflow",
	}
	if len(items) != len(want) {
		t.Fatalf("loaded %d items, want %d: %v", len(items), len(want), urisOf(items))
	}
	for i, uri := range want {
		if items[i].uri != uri {
			t.Errorf("items[%d].uri = %q, want %q", i, items[i].uri, uri)
		}
	}
}

func urisOf(items []systemContribution) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.uri
	}
	return out
}

// TestSystemContributionProvider_EnumerateProducesValidContributions
// asserts the provider wraps loaded items into well-formed
// Contribution envelopes — every required field populated, Body is
// valid JSON, the URI on the envelope matches the URI on the loader
// record.
func TestSystemContributionProvider_EnumerateProducesValidContributions(t *testing.T) {
	items, err := loadSystemContributions()
	if err != nil {
		t.Fatalf("loadSystemContributions: %v", err)
	}
	// All three sample files happen to be different types; group like
	// the loader does and verify each provider.
	byType := map[string][]systemContribution{}
	for _, it := range items {
		byType[it.typ] = append(byType[it.typ], it)
	}
	for typ, group := range byType {
		p := &systemContributionProvider{typ: typ, items: group}
		if p.Type() != typ {
			t.Errorf("provider Type() = %q, want %q", p.Type(), typ)
		}
		if p.Provider() != contribapi.SystemProvider {
			t.Errorf("provider Provider() = %q, want %q", p.Provider(), contribapi.SystemProvider)
		}
		got, err := p.Enumerate(context.Background())
		if err != nil {
			t.Errorf("Enumerate(%q): %v", typ, err)
			continue
		}
		if len(got) != len(group) {
			t.Errorf("Enumerate(%q) returned %d, want %d", typ, len(got), len(group))
		}
		for _, c := range got {
			if c.Provider != contribapi.SystemProvider {
				t.Errorf("Contribution %s Provider = %q", c.URI, c.Provider)
			}
			if c.Type != typ {
				t.Errorf("Contribution %s Type = %q, want %q", c.URI, c.Type, typ)
			}
			if c.ID == "" {
				t.Errorf("Contribution %s ID empty", c.URI)
			}
			if c.URI == "" {
				t.Errorf("Contribution missing URI")
			}
			// Content, in exactly one place, and the right one for the kind.
			//
			// A data-type IS its DSL, so its content is the source layer, as
			// written, and nothing else. Every other system kind is a
			// structured descriptor: its DATA layer is the decoded tree and
			// must decode as JSON; its SOURCE layer is the authored YAML
			// (since 2026-09-22, e7 — the declaration a reader opens). Two
			// layers answering two questions is not one content with two
			// homes, which is what c24 removed: the data-type case above.
			if typ == contribapi.ResourceTypeDataType {
				if len(c.Source()) == 0 {
					t.Errorf("Contribution %s carries no DSL", c.URI)
				}
				if len(c.Data()) != 0 {
					t.Errorf("Contribution %s carries its DSL twice (data layer AND source layer)", c.URI)
				}
				continue
			}
			var tree any
			if err := json.Unmarshal(c.Data(), &tree); err != nil {
				t.Errorf("Contribution %s Body not valid JSON: %v", c.URI, err)
			}
			if len(c.Source()) == 0 {
				t.Errorf("Contribution %s carries no source layer — the authored YAML is the declaration", c.URI)
			}
		}
	}
}

// TestSystemConstants_Roundtrip asserts the URI helpers in system.go
// produce strings that the parser in contribapi accepts. Cheap
// regression net for typos in the helpers.
func TestSystemConstants_Roundtrip(t *testing.T) {
	cases := []struct {
		got, want string
	}{
		{SystemContributionTypeURI("data-type"), "system:/contribution-type/data-type"},
		{SystemDataTypeURI("contribution"), "system:/data-type/contribution"},
		{SystemServiceURI("SkillService"), "system:/service/SkillService"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}
