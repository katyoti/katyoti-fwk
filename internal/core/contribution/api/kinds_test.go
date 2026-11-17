// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func kindEntry(t *testing.T, d KindDeclaration) Contribution {
	t.Helper()
	body, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	c := Contribution{Provider: "ktw-test", Type: ResourceTypeContributionType, ID: d.Name}
	c.SetData(body)
	return c
}

func complete(name string) KindDeclaration {
	return KindDeclaration{
		Name: name, Schema: "ktw-test:/data-type/" + name, Validate: "warn", WarnReason: "a test kind",
		Layers: map[string]LayerDecl{"data": {MediaType: "application/json", Required: true, Default: true}},
		Meta:   &MetaDecl{Budget: 2048},
	}
}

func TestKinds_ResolvesEveryQuestionFromTheDeclaration(t *testing.T) {
	dt := KindDeclaration{Name: "data-type", Schema: "system:/data-type/data-type", Validate: "error",
		Layers: map[string]LayerDecl{"source": {MediaType: "application/yaml", Required: true, Default: true, Resident: true}},
		Meta:   &MetaDecl{Budget: 2048}}
	task := complete("task")
	task.Execution = "executable"
	k := KindsFrom([]Contribution{kindEntry(t, dt), kindEntry(t, task),
		// noise: an entry of another kind, ignored
		{Provider: "ktw-test", Type: "task", ID: "x"}})

	if k.Len() != 2 {
		t.Fatalf("Len = %d, want 2", k.Len())
	}
	if got := k.SchemaFor("task"); got != "ktw-test:/data-type/task" {
		t.Errorf("SchemaFor(task) = %q", got)
	}
	if got := k.DefaultLayer("data-type"); got != LayerSource {
		t.Errorf("DefaultLayer(data-type) = %q, want source", got)
	}
	if got := k.DefaultLayer("task"); got != LayerData {
		t.Errorf("DefaultLayer(task) = %q, want data", got)
	}
	if !k.Resident("data-type", LayerSource) || k.Resident("task", LayerData) || k.Resident("task", LayerSource) {
		t.Error("Resident: only the data-type's source is declared resident")
	}
	if k.Execution("task") != ExecutionExecutable || k.Execution("data-type") != ExecutionDeclarative {
		t.Error("Execution: task is executable, data-type declarative")
	}
	if k.Budget("task") != 2048 || k.Budget("nope") != 0 {
		t.Error("Budget: declared value, 0 when undeclared")
	}
	if k.DeclaredBy("task") != "ktw-test:/contribution-type/task" {
		t.Errorf("DeclaredBy(task) = %q", k.DeclaredBy("task"))
	}
	if len(k.Check()) != 0 {
		t.Errorf("complete declarations must produce no finding, got %v", k.Check())
	}
}

// The meta-model is the base case: undeclared, it resolves to the system
// data-type of the same name; any other undeclared kind resolves to nothing.
func TestKinds_MetaKindsTerminateTheChain(t *testing.T) {
	k := KindsFrom(nil)
	if got := k.SchemaFor(ResourceTypeContributionType); got != "system:/data-type/contribution-type" {
		t.Errorf("SchemaFor(contribution-type) = %q", got)
	}
	if got := k.SchemaFor("skill"); got != "" {
		t.Errorf("SchemaFor(undeclared skill) = %q, want empty", got)
	}
	if got := k.DefaultLayer("skill"); got != LayerSource {
		t.Errorf("DefaultLayer(undeclared) = %q, want the source convention", got)
	}
	if k.Execution("anything") != ExecutionDeclarative {
		t.Error("an undeclared kind is declarative: nothing runs it by accident")
	}
}

// Severity: the KIND declares it; config overrides, per kind then globally;
// nothing else invents a default.
func TestKinds_SeverityComesFromTheKindUnlessOverridden(t *testing.T) {
	strict := complete("strict")
	strict.Validate = "error"
	k := KindsFrom([]Contribution{kindEntry(t, strict), kindEntry(t, complete("lax"))})

	if got := k.SeverityFor("strict", ContributionsConfig{}); got != ValidateOn {
		t.Errorf("kind says error → on, got %q", got)
	}
	if got := k.SeverityFor("lax", ContributionsConfig{}); got != ValidateWarn {
		t.Errorf("kind says warn, got %q", got)
	}
	if got := k.SeverityFor("undeclared", ContributionsConfig{}); got != ValidateWarn {
		t.Errorf("undeclared kind falls to warn, got %q", got)
	}
	if got := k.SeverityFor("strict", ContributionsConfig{Validate: "off"}); got != ValidateOff {
		t.Errorf("global override wins over the kind, got %q", got)
	}
	if got := k.SeverityFor("lax", ContributionsConfig{Validate: "off", PerType: map[string]string{"lax": "error"}}); got != ValidateOn {
		t.Errorf("per-kind override wins over global, got %q", got)
	}
}

func TestKinds_CheckNamesEveryOmission(t *testing.T) {
	noLayers := complete("no-layers")
	noLayers.Layers = nil
	twoDefaults := complete("two-defaults")
	twoDefaults.Layers["source"] = LayerDecl{Default: true}
	noBudget := complete("no-budget")
	noBudget.Meta = nil
	badSeverity := complete("bad-severity")
	badSeverity.Validate = "loud"
	noSchema := complete("no-schema")
	noSchema.Schema = ""

	k := KindsFrom([]Contribution{kindEntry(t, noLayers), kindEntry(t, twoDefaults), kindEntry(t, noBudget),
		kindEntry(t, badSeverity), kindEntry(t, noSchema), kindEntry(t, complete("fine"))})
	findings := k.Check("fine", "ghost", "ghost")

	want := map[string]string{
		"no-layers":    "declares no layers",
		"two-defaults": "2 default layers",
		"no-budget":    "declares no meta budget",
		"bad-severity": "is not off, warn or error",
		"no-schema":    "declares no schema",
		"ghost":        "kind undeclared",
	}
	got := map[string]string{}
	for _, f := range findings {
		got[f.Kind] += f.Message + ";"
		if f.Kind != "ghost" && f.URI == "" {
			t.Errorf("finding on declared kind %q must name the declaring URI", f.Kind)
		}
	}
	for kind, frag := range want {
		if !strings.Contains(got[kind], frag) {
			t.Errorf("kind %q: want a finding containing %q, got %q", kind, frag, got[kind])
		}
	}
	if got["fine"] != "" {
		t.Errorf("complete kind must have no finding, got %q", got["fine"])
	}
	if strings.Count(got["ghost"], "undeclared") != 1 {
		t.Errorf("an undeclared kind used twice is reported once, got %q", got["ghost"])
	}
}

// meta.from hands the picker fields to the framework: the producer writes
// them once, in the named layer, and the builder fills meta. The declaration
// must name a layer the kind has and fields the envelope can derive — a kind
// that says "derive `owner`" would promise a meta field that does not exist.
func TestKinds_MetaFromIsCheckedAgainstTheLayersAndTheEnvelope(t *testing.T) {
	derived := complete("skill")
	derived.Meta = &MetaDecl{Budget: 2048, From: "data", Fields: []string{"name", "description", "icon", "advertised"}}
	noSuchLayer := complete("no-such-layer")
	noSuchLayer.Meta = &MetaDecl{Budget: 1, From: "frontmatter", Fields: []string{"name"}}
	nothingToDerive := complete("nothing-to-derive")
	nothingToDerive.Meta = &MetaDecl{Budget: 1, From: "data"}
	notAMetaField := complete("not-a-meta-field")
	notAMetaField.Meta = &MetaDecl{Budget: 1, From: "data", Fields: []string{"name", "owner"}}

	k := KindsFrom([]Contribution{kindEntry(t, derived), kindEntry(t, noSuchLayer), kindEntry(t, nothingToDerive),
		kindEntry(t, notAMetaField), kindEntry(t, complete("plain"))})

	if layer, fields := k.MetaFrom("skill"); layer != "data" || len(fields) != 4 || fields[2] != "icon" {
		t.Errorf("MetaFrom(skill) = %q %v, want data [name description icon advertised]", layer, fields)
	}
	if layer, fields := k.MetaFrom("plain"); layer != "" || fields != nil {
		t.Errorf("MetaFrom(plain) = %q %v, want none: its producer writes meta", layer, fields)
	}
	if layer, _ := k.MetaFrom("ghost"); layer != "" {
		t.Errorf("MetaFrom of an undeclared kind = %q, want none", layer)
	}

	got := map[string]string{}
	for _, f := range k.Check() {
		got[f.Kind] += f.Message + ";"
	}
	for kind, frag := range map[string]string{
		"no-such-layer":     `names layer "frontmatter", which the kind does not declare`,
		"nothing-to-derive": "meta.fields names nothing to derive",
		"not-a-meta-field":  `names "owner", which is not a derivable meta field`,
	} {
		if !strings.Contains(got[kind], frag) {
			t.Errorf("kind %q: findings %q lack %q", kind, got[kind], frag)
		}
	}
	for _, fine := range []string{"skill", "plain"} {
		if got[fine] != "" {
			t.Errorf("kind %q is complete but was reported: %s", fine, got[fine])
		}
	}
}

// A kind is declared once. Two declarations resolve to ONE — by origin, never
// by order — and every loser is a finding naming both URIs (blind audit H4:
// first-in-slice-order let a member shadow a system kind because the hub lists
// the system carrier last, and the promised finding did not exist).
func TestKinds_SystemDeclarationWinsRegardlessOfOrderAndTheRedeclarationIsAnError(t *testing.T) {
	member := complete("skill")
	member.Schema = "ktw-rogue:/data-type/skill"
	rogue := kindEntry(t, member)
	rogue.Provider = "ktw-rogue"
	sys := complete("skill")
	sys.Schema = "system:/data-type/skill"
	system := kindEntry(t, sys)
	system.Provider = SystemProvider

	for name, order := range map[string][]Contribution{
		"member first": {rogue, system},
		"system first": {system, rogue},
	} {
		k := KindsFrom(order)
		if got := k.SchemaFor("skill"); got != "system:/data-type/skill" {
			t.Errorf("%s: SchemaFor(skill) = %q, want the system declaration to win", name, got)
		}
		if got := k.DeclaredBy("skill"); got != "system:/contribution-type/skill" {
			t.Errorf("%s: DeclaredBy(skill) = %q", name, got)
		}
		if got := k.Declarers("skill"); len(got) != 2 || got[0] != "system:/contribution-type/skill" || got[1] != "ktw-rogue:/contribution-type/skill" {
			t.Errorf("%s: Declarers(skill) = %v, want the system one first, the member's shadowed", name, got)
		}
		findings := k.Check()
		if len(findings) != 1 {
			t.Fatalf("%s: want exactly one finding, got %v", name, findings)
		}
		f := findings[0]
		if f.Rule != KindRuleRedeclaresSystem || f.Severity != ValidateError || f.URI != "ktw-rogue:/contribution-type/skill" {
			t.Errorf("%s: finding = %+v, want an error on the member's URI under %s", name, f, KindRuleRedeclaresSystem)
		}
		if !strings.Contains(f.Message, "system:/contribution-type/skill") || !strings.Contains(f.Message, "may not redeclare") {
			t.Errorf("%s: message must name the system declaration and the rule, got %q", name, f.Message)
		}
	}
}

// Between two members the first in slice order wins — deterministic. The
// same declaration from two members is one kind, no finding (a pack carried
// by several binaries); differing bodies are an error (the universe resolves
// a kind two ways depending on order).
func TestKinds_TwoMembersDeclaringOneKindAreReportedWithBothURIs(t *testing.T) {
	a := kindEntry(t, complete("recipe"))
	a.Provider = "pack-a"
	same := kindEntry(t, complete("recipe"))
	same.Provider = "pack-b"
	differs := complete("recipe")
	differs.Validate = "error"
	other := kindEntry(t, differs)
	other.Provider = "pack-c"

	k := KindsFrom([]Contribution{a, same, other})
	if got := k.DeclaredBy("recipe"); got != "pack-a:/contribution-type/recipe" {
		t.Errorf("DeclaredBy = %q, want the first member's", got)
	}
	if k.Len() != 1 {
		t.Errorf("Len = %d, want 1: one kind, however many declarations", k.Len())
	}
	byURI := map[string]KindFinding{}
	for _, f := range k.Check() {
		byURI[f.URI] = f
	}
	if len(byURI) != 1 {
		t.Fatalf("want one finding, for the differing declaration only, got %v", byURI)
	}
	if got := k.Declarers("recipe"); len(got) != 3 {
		t.Errorf("Declarers = %v, want all three", got)
	}
	if f := byURI["pack-c:/contribution-type/recipe"]; f.Rule != KindRuleDuplicate || f.Severity != ValidateError ||
		!strings.Contains(f.Message, "different") || !strings.Contains(f.Message, "pack-a:/contribution-type/recipe") {
		t.Errorf("differing duplicate: %+v, want an error naming the resolved URI", f)
	}
	// The same URI met twice is one entry, not a second declaration.
	if k2 := KindsFrom([]Contribution{a, a}); len(k2.Check()) != 0 || len(k2.Declarers("recipe")) != 1 {
		t.Errorf("one URI twice must be one declaration, got findings %v declarers %v", k2.Check(), k2.Declarers("recipe"))
	}
	// Every finding carries a rule and a severity, the old ones included.
	for _, f := range KindsFrom([]Contribution{kindEntry(t, KindDeclaration{Name: "bare"})}).Check("ghost") {
		if f.Rule == "" || f.Severity == "" {
			t.Errorf("finding without rule or severity: %+v", f)
		}
	}
}

// A pack that declares its kind is carried by several binaries: each
// publishes the declaration under its own provider, naming the schema it
// ships under that same provider. Bodies differing only by that provider are
// one kind and no finding; any other difference is a kind-duplicate error
// naming both URIs.
func TestKinds_OnePackDeclarationCarriedByTwoBinariesIsOneKind(t *testing.T) {
	carried := func(provider string, edit func(*KindDeclaration)) Contribution {
		d := complete("skill")
		d.Schema = provider + ":/data-type/skill"
		if edit != nil {
			edit(&d)
		}
		c := kindEntry(t, d)
		c.Provider = provider
		return c
	}
	k := KindsFrom([]Contribution{carried("ktw-agents", nil), carried("ktw-gocode", nil)})
	if k.Len() != 1 || len(k.Check()) != 0 {
		t.Fatalf("Len = %d, findings %v: want one kind, no finding", k.Len(), k.Check())
	}
	if got := k.SchemaFor("skill"); got != "ktw-agents:/data-type/skill" {
		t.Errorf("SchemaFor = %q, want the resolved declarer's schema", got)
	}
	if got := k.Declarers("skill"); len(got) != 2 || got[0] != "ktw-agents:/contribution-type/skill" {
		t.Errorf("Declarers = %v, want both, the resolved first", got)
	}
	if got := k.AgreeingSchemas("skill"); len(got) != 2 || got[0].Schema != "ktw-agents:/data-type/skill" ||
		got[1].Schema != "ktw-gocode:/data-type/skill" {
		t.Errorf("AgreeingSchemas = %v, want each declarer's own schema, the resolved first", got)
	}

	// Naming the same schema URI is the same declaration too; naming another
	// binary's schema under a name of one's own is not.
	if fs := KindsFrom([]Contribution{carried("ktw-agents", nil),
		carried("ktw-gocode", func(d *KindDeclaration) { d.Schema = "ktw-agents:/data-type/skill" })}).Check(); len(fs) != 0 {
		t.Errorf("both naming ktw-agents' schema: findings %v, want none", fs)
	}
	if fs := KindsFrom([]Contribution{carried("ktw-agents", nil),
		carried("ktw-gocode", func(d *KindDeclaration) { d.Schema = "ktw-other:/data-type/skill" })}).Check(); len(fs) != 1 || fs[0].Rule != KindRuleDuplicate {
		t.Errorf("own vs another provider's schema: findings %v, want one kind-duplicate", fs)
	}

	differs := KindsFrom([]Contribution{carried("ktw-agents", nil),
		carried("ktw-gocode", func(d *KindDeclaration) { d.Validate = "error" })})
	if got := differs.AgreeingSchemas("skill"); got != nil {
		t.Errorf("AgreeingSchemas of differing declarations = %v, want nil", got)
	}
	fs := differs.Check()
	if len(fs) != 1 || fs[0].Rule != KindRuleDuplicate || fs[0].Severity != ValidateError ||
		!strings.Contains(fs[0].Message, "ktw-agents:/contribution-type/skill") ||
		!strings.Contains(fs[0].Message, "ktw-gocode:/contribution-type/skill") {
		t.Errorf("one field changed: findings %v, want one kind-duplicate error naming both URIs", fs)
	}
}

// Every kind validates its entries as an error: a kind kept at warn states
// why (warn_reason), and a reason on a kind not at warn is refused too - each
// under its own rule, so the census can tell it apart.
func TestKinds_CheckRefusesAWarnWithoutItsReason(t *testing.T) {
	bare := complete("bare-warn")
	bare.WarnReason = ""
	kept := complete("kept-warn")
	kept.WarnReason = "its bodies are free text no schema describes"
	stray := complete("stray-reason")
	stray.Validate = ValidateError
	strict := complete("strict")
	strict.Validate, strict.WarnReason = ValidateError, ""

	k := KindsFrom([]Contribution{kindEntry(t, bare), kindEntry(t, kept), kindEntry(t, stray), kindEntry(t, strict)})
	got := map[string]KindFinding{}
	for _, f := range k.Check() {
		got[f.Kind] = f
	}
	for _, name := range []string{"bare-warn", "stray-reason"} {
		f, ok := got[name]
		if !ok {
			t.Errorf("%s: no finding, want %s", name, KindRuleWarnUnexplained)
			continue
		}
		if f.Rule != KindRuleWarnUnexplained || f.Severity != ValidateError {
			t.Errorf("%s: rule %q severity %q, want %q error", name, f.Rule, f.Severity, KindRuleWarnUnexplained)
		}
	}
	for _, name := range []string{"kept-warn", "strict"} {
		if f, ok := got[name]; ok {
			t.Errorf("%s: unexpected finding %s", name, f)
		}
	}
}
