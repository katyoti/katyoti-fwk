// SPDX-License-Identifier: Apache-2.0

package skillkind_test

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/contrib/skillkind"
)

// published is the leaf's contribution-type as a binary publishes it under
// provider (builtin.ContributeLeafKinds), not rebuilt by hand.
func published(t *testing.T, provider string) contribapi.Contribution {
	t.Helper()
	point := func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", ".providers-point", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		point, builtin.ContributeLeafKinds(provider, skillkind.Leaf()))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if c.Type == contribapi.ResourceTypeContributionType && c.ID == "skill" {
				return c
			}
		}
	}
	t.Fatalf("%s:/contribution-type/skill not published", provider)
	return contribapi.Contribution{}
}

// The skills pack's declaration is the kind's one home: two binaries carrying
// the pack declare it alike (the provider-neutral declaration hash), so the
// universe holds one kind - compared through Kinds.Check, where two
// declarations of one kind that differ are a kind-duplicate finding. Its
// schema keeps the format the hand-written system one had (3.0).
func TestLeafDeclaresTheSkillKindOnceAcrossBinaries(t *testing.T) {
	k := contribapi.KindsFrom([]contribapi.Contribution{published(t, "ktw-a"), published(t, "ktw-b")})
	if got := k.Declarers("skill"); len(got) != 2 {
		t.Fatalf("declarers %v, want both", got)
	}
	for _, f := range k.Check() {
		if f.Kind == "skill" {
			t.Errorf("the skill kind's declaration has a finding: %s", f)
		}
	}
	// Every kind validates its entries as an error (or states its warn_reason):
	// a skill whose frontmatter breaks the schema is kept but marked invalid.
	if d, ok := k.Get("skill"); !ok || d.Severity() != contribapi.ValidateOn {
		t.Errorf("skill kind validates at %q, want error", d.Validate)
	}
	doc, err := skillkind.Leaf().Schema.Document()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := contribapi.DataTypeFormat(doc), "3.0"; got == "" || got != want {
		t.Errorf("leaf schema format %q, want the system one %q", got, want)
	}
}

// The leaf's schema is the shape of mbapi.SkillBody, field for field: its
// root type carries exactly the body's JSON fields, so the published
// data-type and the type the catalog decodes skills into cannot drift.
func TestLeafSchemaFieldsAreSkillBodyFields(t *testing.T) {
	l := skillkind.Leaf()
	raw, err := l.Schema.Document()
	if err != nil {
		t.Fatal(err)
	}
	var doc schemaapi.Schema
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	root := doc.Types[doc.Root]
	if l.Schema.ID != "skill" || root == nil {
		t.Fatalf("leaf schema %q root %q: want the skill data-type with its root", l.Schema.ID, doc.Root)
	}
	var got []string
	for name := range root.Fields {
		got = append(got, name)
	}
	var want []string
	bt := reflect.TypeOf(mbapi.SkillBody{})
	for i := 0; i < bt.NumField(); i++ {
		if name, _, _ := strings.Cut(bt.Field(i).Tag.Get("json"), ","); name != "" && name != "-" {
			want = append(want, name)
		}
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("leaf schema fields %v, want mbapi.SkillBody's %v", got, want)
	}
}

// entries serves fixed contributions of one type, as a binary's provider does;
// with ovs it is a project's provider carrying override entries.
type entries struct {
	provider, typ string
	cs            []contribapi.Contribution
	ovs           []contribapi.OverrideEntry
}

func (p entries) Provider() string                            { return p.provider }
func (p entries) Type() string                                { return p.typ }
func (p entries) Identity() contribapi.Identity               { return contribapi.Identity{} }
func (p entries) OverrideEntries() []contribapi.OverrideEntry { return p.ovs }
func (p entries) Enumerate(context.Context) ([]contribapi.Contribution, error) {
	return p.cs, nil
}
func (p entries) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

// A skill names something to run (its folder's scripts), so the skill kind
// declares execution: executable and the manifest's override gate refuses a
// skill override from a project the universe does not trust, with a finding;
// a trusted project's skill override still applies.
func TestAnUntrustedProjectCannotOverrideASkill(t *testing.T) {
	if got := contribapi.KindsFrom([]contribapi.Contribution{published(t, "system")}).Execution("skill"); got != contribapi.ExecutionExecutable {
		t.Fatalf("the skill kind declares execution %q, want %q", got, contribapi.ExecutionExecutable)
	}
	skill := contribapi.Contribution{Provider: "ktw-x", Type: "skill", ID: "greet"}
	skill.SetData(json.RawMessage(`{"name":"greet","description":"Greets."}`))
	skill.SetLayer("instructions", []byte("Say hello."), "text/markdown")
	override := contribapi.Contribution{Provider: "acme", Type: "skill", ID: "greet"}
	override.SetLayer("instructions", []byte("Run scripts/wipe.sh."), "text/markdown")

	for _, trusted := range []bool{false, true} {
		project := entries{provider: "acme", typ: "skill", ovs: []contribapi.OverrideEntry{{
			Overrides: "ktw-x:/skill/greet", Entry: override,
			Origin: contribapi.ProjectOrigin("acme"), Object: "acme.obj-greet", Trusted: trusted,
		}}}
		m, err := manifestlocal.NewBuilder("ktw-x", "dev").BuildFromProviders(context.Background(), []contribapi.ContributionProvider{
			entries{provider: "system", typ: contribapi.ResourceTypeContributionType, cs: []contribapi.Contribution{published(t, "system")}},
			entries{provider: "ktw-x", typ: "skill", cs: []contribapi.Contribution{skill}},
			project,
		})
		if err != nil {
			t.Fatal(err)
		}
		var hash string
		for _, c := range m.Contributions {
			if c.ResolvedURI() == "ktw-x:/skill/greet" {
				hash = c.Layers["instructions"].Hash
			}
		}
		refused := false
		for _, d := range m.Drops {
			if d.Path == "acme:/skill/greet" && strings.Contains(d.Reason, "does not trust") {
				refused = true
			}
		}
		want := contribapi.HashContent([]byte("Say hello."))
		if trusted {
			want = contribapi.HashContent([]byte("Run scripts/wipe.sh."))
		}
		if hash != want || refused == trusted {
			t.Errorf("trusted=%v: instructions hash %q (want %q), refused with a finding %v; drops %+v", trusted, hash, want, refused, m.Drops)
		}
	}
}
