// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A gate requirement selects by shape through `applies` (a ShapeFilter): a
// match, a miss answering why, and the empty filter applying to every object
// through the gate.
func TestGateRequirementAppliesBySelector(t *testing.T) {
	var r GateRequirement
	if err := yaml.Unmarshal([]byte("name: specified\ngate: ready-to-plan\napplies: {types: [feature], capabilities: [want]}\ncheck: \"true\"\n"), &r); err != nil {
		t.Fatal(err)
	}
	if ok, why := r.AppliesTo(cwmapi.FilterSubject{Type: "feature", Capabilities: []string{"want"}}); !ok {
		t.Errorf("must apply to a feature carrying want: %s", why)
	}
	if ok, why := r.AppliesTo(cwmapi.FilterSubject{Type: "goal", Capabilities: []string{"want"}}); ok || !strings.Contains(why, `type "goal"`) {
		t.Errorf("must miss a goal, naming its type: %v %q", ok, why)
	}
	if ok, why := r.AppliesTo(cwmapi.FilterSubject{Type: "feature"}); ok || !strings.Contains(why, "carries none of [want]") {
		t.Errorf("must miss a feature without want, saying so: %v %q", ok, why)
	}
	if ok, _ := (GateRequirement{Name: "any"}).AppliesTo(cwmapi.FilterSubject{Type: "goal"}); !ok {
		t.Error("an empty applies must apply to every object through the gate")
	}
}

// The old `types:` key is refused, naming it, in JSON and in YAML — left in a
// pack it would otherwise decode into nothing and apply the requirement to
// every object; a content field in `applies` is refused too.
func TestGateRequirementRefusesTheOldKeyAndAContentFilter(t *testing.T) {
	var r GateRequirement
	if err := yaml.Unmarshal([]byte("name: x\ngate: g\ntypes: [feature]\ncheck: \"true\"\n"), &r); err == nil || !strings.Contains(err.Error(), "types is no field") {
		t.Errorf("YAML: the old types key must be refused, got %v", err)
	}
	if err := json.Unmarshal([]byte(`{"name":"x","gate":"g","types":["feature"],"check":"true"}`), &r); err == nil || !strings.Contains(err.Error(), "types is no field") {
		t.Errorf("JSON: the old types key must be refused, got %v", err)
	}
	if err := json.Unmarshal([]byte(`{"name":"x","gate":"g","applies":{"when":"true"},"check":"true"}`), &r); err == nil || !strings.Contains(err.Error(), "when refused") {
		t.Errorf("a when in applies must be refused, got %v", err)
	}
	var g Gate
	if err := yaml.Unmarshal([]byte("name: g\nrequirements:\n  - {name: x, types: [feature], check: \"true\"}\n"), &g); err == nil || !strings.Contains(err.Error(), "types is no field") {
		t.Errorf("a gate's own requirement must refuse the old key too, got %v", err)
	}
}

// A requirement's step (resolve.step) is read strictly: a valid one decodes;
// a misspelt key, both role and interaction, or neither, is refused naming
// the requirement — by Step and by Validate, so the gate cannot pass on it;
// a requirement with no step is unchanged.
func TestARequirementsStepIsReadStrictly(t *testing.T) {
	req := func(step string) GateRequirement {
		t.Helper()
		var r GateRequirement
		if err := yaml.Unmarshal([]byte("name: tested\ngate: ready-to-test\ncheck: \"true\"\nresolve:\n  title: Test {object}\n"+step), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	ok := req("  step: {name: write-tests, role: tester, skill: k-test, instructions: Cover it, phase: build, priority: 2}\n")
	st, has, err := ok.Step()
	want := RequirementStep{Name: "write-tests", Role: "tester", Skill: "k-test", Instructions: "Cover it", Phase: "build", Priority: 2}
	if err != nil || !has || st != want || ok.Validate() != nil {
		t.Errorf("a valid step: %+v %v %v (validate %v)", st, has, err, ok.Validate())
	}
	if st, has, err := req("  step: {name: review, interaction: validate}\n").Step(); err != nil || !has || st.Interaction != InteractionValidate {
		t.Errorf("a person's step: %+v %v %v", st, has, err)
	}
	for _, c := range []struct{ step, why string }{
		{"  step: {name: x, rol: tester}\n", `unknown field "rol"`},
		{"  step: {name: x, role: tester, interaction: validate}\n", "exactly one of role, interaction"},
		{"  step: {name: x, skill: k-test}\n", "exactly one of role, interaction"},
		{"  step: {name: x, interaction: approve}\n", `interaction "approve"`},
	} {
		step, why := c.step, c.why
		r := req(step)
		_, has, err := r.Step()
		if !has || err == nil || !strings.Contains(err.Error(), why) || !strings.Contains(err.Error(), `"tested"`) {
			t.Errorf("%s: must be refused naming the requirement, got %v", step, err)
		}
		if err := r.Validate(); err == nil || !strings.Contains(err.Error(), why) {
			t.Errorf("%s: Validate must refuse it too, got %v", step, err)
		}
	}
	none := req("")
	if _, has, err := none.Step(); has || err != nil || none.Validate() != nil || none.Resolve["title"] != "Test {object}" {
		t.Errorf("no step: %v %v %v %+v", has, err, none.Validate(), none.Resolve)
	}
}
