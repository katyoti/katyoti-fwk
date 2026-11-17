// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	oclocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// EFFECTIVE — which objects of a kind are in force at an object. The tree is
// project → module → feature, with a rule at each level; the declared rules
// DECIDE the answer: change the propagation and the set changes; change the
// strategy and the same sources reduce differently.

func effectiveWorld(t *testing.T, ruleType string) (context.Context, cwmapi.CWMService, api.ObjectContextService) {
	t.Helper()
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"capabilities/container.yaml": {Data: []byte("name: container\n")},
		"capabilities/content.yaml":   {Data: []byte("name: content\n")},
		"capabilities/policy.yaml":    {Data: []byte("name: policy\n")},
		"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container]\n")},
		"object-types/project.yaml":   {Data: []byte("name: project\ncapabilities: [container]\ntree: node\n")},
		"object-types/area.yaml":      {Data: []byte("name: area\ncapabilities: [container]\ntree: node\n")},
		"object-types/feature.yaml":   {Data: []byte("name: feature\ncapabilities: [container]\ntree: node\n")},
		"object-types/rule.yaml":      {Data: []byte("name: rule\ncapabilities: [container, content, policy]\n" + ruleType)},
	}), oclocal.ExtensionWith(api.ObjectContextConfig{}))
	cwmapp.Module(t, ctx, cwm, "m")
	mk := func(id, typ, parent string, group map[string]any) {
		o := cwmapi.Object{ID: id, Type: typ}
		o.Name, o.Parent = id, parent
		if group != nil {
			o.Capabilities = map[string]map[string]any{"policy": group}
		}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("m.proj", "project", "m", nil)
	mk("m.mod", "area", "m.proj", nil)
	mk("m.feat", "feature", "m.mod", nil)
	// A rule at each level; two share a `topic`, which the override key keys on.
	mk("m.r-proj", "rule", "m.proj", map[string]any{"topic": "tests"})
	mk("m.r-mod", "rule", "m.mod", map[string]any{"topic": "tests"})
	mk("m.r-feat", "rule", "m.feat", map[string]any{"topic": "style"})
	oc, err := application.UseService[api.ObjectContextService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, oc
}

// ruleType is the rule type's `context:` section — the settings
// object-context reads (api.ContextOf).
func ruleType(propagation, strategy, key string) string {
	var b strings.Builder
	b.WriteString("context:\n")
	for k, v := range map[string]string{"propagation": propagation, "resolution_strategy": strategy, "override_key": key} {
		if v != "" {
			b.WriteString("  " + k + ": " + v + "\n")
		}
	}
	return b.String()
}

func ids(out api.EffectiveOutput) []string {
	var out2 []string
	for _, o := range out.Objects {
		out2 = append(out2, o.ID)
	}
	return out2
}

func effective(t *testing.T, ctx context.Context, oc api.ObjectContextService, in api.EffectiveInput) api.EffectiveOutput {
	t.Helper()
	out, err := oc.Effective(ctx, in)
	if err != nil {
		t.Fatalf("Effective %+v: %v", in, err)
	}
	return out
}

func TestEffectiveInheritsFromAncestors(t *testing.T) {
	ctx, _, oc := effectiveWorld(t, ruleType(api.PropagationInheritFromAncestors, "accumulate", ""))
	out := effective(t, ctx, oc, api.EffectiveInput{ID: "m.feat", Type: "rule", IncludeBy: true})
	got := ids(out)
	if len(got) != 3 || got[0] != "m.r-feat" || !slices.Contains(got, "m.r-mod") || !slices.Contains(got, "m.r-proj") {
		t.Fatalf("ids = %v, want the three, the local one first", got)
	}
	if out.BySource == nil || len(out.BySource.Local) != 1 || len(out.BySource.Tree) != 2 {
		t.Errorf("by-source: %+v", out.BySource)
	}
	if out.Propagation != api.PropagationInheritFromAncestors || out.Strategy != "accumulate" {
		t.Errorf("the applied rules must be echoed: %+v", out)
	}
}

func TestEffectiveLocalReachesNothing(t *testing.T) {
	ctx, _, oc := effectiveWorld(t, ruleType(api.PropagationLocal, "accumulate", ""))
	if got := ids(effective(t, ctx, oc, api.EffectiveInput{ID: "m.feat", Type: "rule"})); len(got) != 1 || got[0] != "m.r-feat" {
		t.Errorf("local: %v", got)
	}
}

func TestEffectivePropagatesToAncestors(t *testing.T) {
	ctx, _, oc := effectiveWorld(t, ruleType(api.PropagationPropagateToAncestors, "accumulate", ""))
	if got := ids(effective(t, ctx, oc, api.EffectiveInput{ID: "m.proj", Type: "rule"})); len(got) != 3 || got[0] != "m.r-proj" {
		t.Errorf("descendant walk: %v", got)
	}
}

func TestEffectiveClosestWinsOnTheOverrideKey(t *testing.T) {
	ctx, _, oc := effectiveWorld(t, ruleType(api.PropagationInheritFromAncestors, "closest-wins", "topic"))
	out := effective(t, ctx, oc, api.EffectiveInput{ID: "m.feat", Type: "rule"})
	got := ids(out)
	if slices.Contains(got, "m.r-proj") || !slices.Contains(got, "m.r-mod") || !slices.Contains(got, "m.r-feat") {
		t.Errorf("closest-wins on topic: %v", got)
	}
	for _, o := range out.Objects {
		if o.ID == "m.r-mod" && o.StructuredFields["topic"] != "tests" {
			t.Errorf("the override key was not read from the capability group: %+v", o.StructuredFields)
		}
	}
}

func TestEffectiveDefaultsAreTheQuietOnes(t *testing.T) {
	ctx, _, oc := effectiveWorld(t, "")
	out := effective(t, ctx, oc, api.EffectiveInput{ID: "m.feat", Type: "rule"})
	if len(out.Objects) != 1 || out.Propagation != api.DefaultPropagation || out.Strategy != api.DefaultResolutionStrategy {
		t.Errorf("defaults: %+v", out)
	}
}

func TestEffectiveRefusesWhatItCannotEvaluate(t *testing.T) {
	ctx, _, oc := effectiveWorld(t, ruleType(api.PropagationLocal, "accumulate", ""))
	if _, err := oc.Effective(ctx, api.EffectiveInput{ID: "m.feat", Type: "ghost"}); err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Errorf("an undeclared kind: %v", err)
	}
	ctx2, _, oc2 := effectiveWorld(t, ruleType(api.PropagationPerCriterion, api.ResolutionPerCriterionOverride, ""))
	if _, err := oc2.Effective(ctx2, api.EffectiveInput{ID: "m.feat", Type: "rule"}); !errors.Is(err, cwmapi.ErrNotImplemented) {
		t.Errorf("per-criterion-override: %v", err)
	}
	ctx3, _, oc3 := effectiveWorld(t, ruleType(api.PropagationLocal, "invented-strategy", ""))
	if _, err := oc3.Effective(ctx3, api.EffectiveInput{ID: "m.feat", Type: "rule"}); err == nil || !strings.Contains(err.Error(), "accumulate") {
		t.Errorf("an unknown strategy must name the ones that exist: %v", err)
	}
}

func TestEffectiveSurvivesAParentCycle(t *testing.T) {
	ctx, cwm, oc := effectiveWorld(t, ruleType(api.PropagationInheritFromAncestors, "accumulate", ""))
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.proj"})
	if err != nil {
		t.Fatal(err)
	}
	proj := got.Object
	proj.Parent = "m.feat"
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: proj}); err != nil {
		t.Skipf("CWM refuses the cycle at write: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = oc.Effective(ctx, api.EffectiveInput{ID: "m.feat", Type: "rule"})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the ancestor walk did not terminate on a parent cycle")
	}
}

func TestEffectiveReadsTheOverrideKeyFromTheFieldsToo(t *testing.T) {
	ctx, cwm, oc := effectiveWorld(t, ruleType(api.PropagationLocal, "closest-wins", "topic"))
	o := cwmapi.Object{ID: "m.r-body", Type: "rule"}
	o.Name, o.Parent = "body rule", "m.feat"
	body, _ := json.Marshal(map[string]any{"topic": "from-body"})
	o.SetFieldsJSON(body)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	for _, b := range effective(t, ctx, oc, api.EffectiveInput{ID: "m.feat", Type: "rule"}).Objects {
		if b.ID == "m.r-body" && b.StructuredFields["topic"] != "from-body" {
			t.Errorf("the override key in the fields was not read: %+v", b.StructuredFields)
		}
	}
}
