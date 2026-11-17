// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
)

// Every object-context setting of a type is read from its `context:` section:
// here `briefing_fields` carries a field of each rule into its brief, with the
// propagation and strategy beside it — none of them a field of the type.
func TestTheContextSectionBriefsTheRules(t *testing.T) {
	ctx, _, oc := effectiveWorld(t, "context:\n  propagation: local\n  resolution_strategy: accumulate\n  briefing_fields: [topic]\n")
	out := effective(t, ctx, oc, api.EffectiveInput{ID: "m.feat", Type: "rule"})
	if out.Propagation != api.PropagationLocal || out.Strategy != "accumulate" {
		t.Fatalf("rules = %s / %s, want the section's local / accumulate", out.Propagation, out.Strategy)
	}
	if len(out.Objects) != 1 || out.Objects[0].ID != "m.r-feat" {
		t.Fatalf("objects = %v, want the feature's own rule", ids(out))
	}
	if got := out.Objects[0].StructuredFields["topic"]; got != "style" {
		t.Errorf("briefing_fields not read from the section: brief = %+v", out.Objects[0])
	}
}
