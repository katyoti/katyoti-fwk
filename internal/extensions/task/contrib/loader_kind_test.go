// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

func TestParse_KindDefaultsToGlobal(t *testing.T) {
	data := []byte(`---
name: legacy-task
type: simulator
---
body`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Kind != api.KindGlobal {
		t.Errorf("Kind = %q, want %q (default for tasks without explicit kind:)", got.Kind, api.KindGlobal)
	}
	if got.Types != nil {
		t.Errorf("Types = %v, want nil for global tasks", got.Types)
	}
}

func TestParse_KindObjectWithTypes(t *testing.T) {
	data := []byte(`---
name: review-document
type: agent
kind: object
types: [document, design-doc]
description: Review a CWM document
---
body`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Kind != api.KindObject {
		t.Errorf("Kind = %q, want %q", got.Kind, api.KindObject)
	}
	want := []string{"document", "design-doc"}
	if len(got.Types) != len(want) {
		t.Fatalf("Types = %v, want %v", got.Types, want)
	}
	for i, w := range want {
		if got.Types[i] != w {
			t.Errorf("Types[%d] = %q, want %q", i, got.Types[i], w)
		}
	}
}

func TestParse_KindObjectWithoutTypesErrors(t *testing.T) {
	data := []byte(`---
name: bad-object-task
type: agent
kind: object
---
body`)
	if _, err := Parse(data); err == nil {
		t.Error("expected validation error: kind=object requires a types list")
	}
}

func TestParse_UnknownKindErrors(t *testing.T) {
	data := []byte(`---
name: bogus
type: agent
kind: enterprise
---
body`)
	if _, err := Parse(data); err == nil {
		t.Error("expected validation error: unknown kind 'enterprise'")
	}
}

func TestParse_KindAndTypesAreMetaNotConfig(t *testing.T) {
	// kind + types are framework metadata; they MUST NOT leak into the
	// strategy's Config map (which would expose them to the strategy as if
	// they were strategy parameters).
	data := []byte(`---
name: doc-review
type: agent
kind: object
types: [document]
capability: smart-best
---
body`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, present := got.Config["kind"]; present {
		t.Error("kind leaked into Config — must be a meta field")
	}
	if _, present := got.Config["types"]; present {
		t.Error("types leaked into Config — must be a meta field")
	}
	// Legacy non-meta fields still flow through to Config.
	if got.Config["capability"] != "smart-best" {
		t.Errorf("legacy 'capability' lost: %v", got.Config)
	}
}

func TestParse_GlobalKindIgnoresTypes(t *testing.T) {
	// types on a global task is permitted (no error) but semantically
	// meaningless — record what the parser does.
	data := []byte(`---
name: hybrid
type: agent
types: [document]
---
body`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Kind != api.KindGlobal {
		t.Errorf("Kind = %q, want %q", got.Kind, api.KindGlobal)
	}
	// types is parsed for completeness; consumers ignore it for global tasks.
	if len(got.Types) != 1 || got.Types[0] != "document" {
		t.Errorf("Types = %v, want [document] (parsed but ignored at runtime)", got.Types)
	}
}

func TestIsKnownKind(t *testing.T) {
	cases := map[string]bool{
		"":           true, // empty = default global, accepted
		"global":     true,
		"object":     true,
		"collection": true,
		"node":       false,
		"item":       false,
		"workspace":  false,
		"enterprise": false,
	}
	for kind, want := range cases {
		if got := api.IsKnownKind(kind); got != want {
			t.Errorf("IsKnownKind(%q) = %v, want %v", kind, got, want)
		}
	}
}

func TestParse_NodeItemVocabularyIsRefusedByName(t *testing.T) {
	for _, fm := range []string{
		"kind: node\ntypes: [document]",
		"kind: item\ntypes: [goal]",
		"kind: item-collection\ntypes: [goal]",
		"kind: object\nnode_types: [document]",
		"kind: object\nitemTypes: [goal]",
	} {
		data := []byte("---\nname: old\ntype: agent\n" + fm + "\n---\nbody")
		_, err := Parse(data)
		if err == nil || !strings.Contains(err.Error(), "vocabulary") {
			t.Errorf("%q: err = %v, want a refusal naming the new key", fm, err)
		}
	}
}
