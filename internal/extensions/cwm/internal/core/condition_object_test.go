// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

func objWithCaps(id, typ, parent string) api.Object {
	o := api.Object{ID: id, Type: typ}
	o.Parent = parent
	o.Name = "A Thing"
	o.Slug = "a-thing"
	o.Name = "T"
	o.Tags = []string{"urgent"}
	o.SetLifecycle(api.Lifecycle{State: "done"})
	return o
}

// The binding taken for cwm.p1r: capability groups under `object`, identity as
// top-level scalars.
func TestObjectWhenReadsCapabilityFields(t *testing.T) {
	o := objWithCaps("cwm.x", "use-case", "cwm.p1")

	cases := []struct {
		expr         string
		wantDisabled bool
	}{
		{`object.lifecycle.state == 'done'`, false},
		{`object.lifecycle.state == 'pending'`, true},
		{`object.name == 'T'`, false},
		{`object.slug == 'a-thing'`, false},
		{`'urgent' in object.tags`, false},
		{`'calm' in object.tags`, true},
		{`id == 'cwm.x'`, false},
		{`type == 'use-case'`, false},
		{`parent_id == 'cwm.p1'`, false},
		{`parent_id == 'somewhere-else'`, true},
		// Identity and capability in one predicate — the shape a real guard takes.
		{`type == 'use-case' && object.lifecycle.state == 'done'`, false},
	}
	for _, c := range cases {
		disabled, reason := evalObjectActionWhen(c.expr, o)
		if disabled != c.wantDisabled {
			t.Errorf("%s: disabled=%v want %v (%s)", c.expr, disabled, c.wantDisabled, reason)
		}
	}
}

// A capability the object does NOT carry must not blow up the guard — it is a
// perfectly ordinary question to ask ("is this checkable and core?") of an
// object that is neither.
func TestObjectWhenHandlesAbsentCapability(t *testing.T) {
	o := objWithCaps("cwm.x", "use-case", "")
	disabled, reason := evalObjectActionWhen(`'checkable' in object && object.checkable.severity == 'core'`, o)
	if !disabled {
		t.Errorf("absent capability read as a match (%s)", reason)
	}
	if !strings.HasPrefix(reason, "condition not met") {
		t.Errorf("absent capability must read FALSE, not fail to compile or evaluate: %s", reason)
	}
}

// THE FAILURE POSTURE, and the one that matters most: a broken guard must
// DISABLE, never enable. The whole point of a `when:` is to withhold an action,
// so a predicate that cannot be judged has to withhold it.
func TestBrokenObjectPredicateDisables(t *testing.T) {
	o := objWithCaps("cwm.x", "use-case", "")
	for _, expr := range []string{
		`this is not el (((`,     // compile error
		`object.lifecycle.state`, // not a bool
		`1 + 1`,                  // not a bool
	} {
		disabled, reason := evalObjectActionWhen(expr, o)
		if !disabled {
			t.Errorf("%q did not disable the action — a broken guard read as permission", expr)
		}
		if reason == "" {
			t.Errorf("%q disabled with no reason", expr)
		}
	}
}

// A not-yet-migrated `node.*` predicate must disable its action AND say that
// the predicate still speaks the node vocabulary.
//
// The first version of this test asserted the opposite — that the predicate
// should read falsy rather than error — and the implementation bound `node` to
// an empty map to achieve it. Measured, that produced "no such key:
// type_specific": a FIELD name, which hides that the whole surface is gone.
// Leaving `node` unbound produces "no such attribute(s): node", which names the
// VOCABULARY. Both disable; only the second says what to fix, so the empty
// binding was removed and this test now pins the better message.
func TestLegacyNodePredicateNamesTheOldVocabulary(t *testing.T) {
	o := objWithCaps("cwm.x", "use-case", "")
	disabled, reason := evalObjectActionWhen(`node.type_specific.status == 'ready'`, o)
	if !disabled {
		t.Fatal("a legacy node.* predicate enabled the action")
	}
	if !strings.Contains(reason, "node") {
		t.Errorf("reason does not name the stale vocabulary, so a reader cannot tell what to migrate: %s", reason)
	}
}

// A hyphenated capability reads with underscores in an expression — a hyphen
// would end the name there.
func TestAHyphenatedGroupReadsWithUnderscores(t *testing.T) {
	o := objWithCaps("cwm.x", "use-case", "cwm.p1")
	_ = o.SetCapability("module-health", map[string]any{"score": 3})
	if disabled, reason := evalObjectActionWhen(`object.module_health.score == 3`, o); disabled {
		t.Fatalf("object.module_health.score: %s", reason)
	}
	if disabled, _ := evalObjectActionWhen(`object.module_health.score == 4`, o); !disabled {
		t.Fatal("the alias does not read the group")
	}
}
