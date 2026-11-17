// SPDX-License-Identifier: Apache-2.0

package personakind_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/personas/contrib/personakind"
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
		point, builtin.ContributeLeafKinds(provider, personakind.Leaf()))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if c.Type == contribapi.ResourceTypeContributionType && c.ID == "persona" {
				return c
			}
		}
	}
	t.Fatalf("%s:/contribution-type/persona not published", provider)
	return contribapi.Contribution{}
}

// The personas pack's declaration is the kind's one home: two binaries carrying
// the pack declare it alike (the provider-neutral declaration hash), so the
// universe holds one kind - compared through Kinds.Check, where two
// declarations of one kind that differ are a kind-duplicate finding. Its
// schema is a newer minor of the hand-written 1.1: a reader of 1.1 reads it.
func TestLeafDeclaresThePersonaKindOnceAcrossBinaries(t *testing.T) {
	k := contribapi.KindsFrom([]contribapi.Contribution{published(t, "ktw-a"), published(t, "ktw-b")})
	if got := k.Declarers("persona"); len(got) != 2 {
		t.Fatalf("declarers %v, want both", got)
	}
	for _, f := range k.Check() {
		if f.Kind == "persona" {
			t.Errorf("two binaries' declarations of the kind differ: %s", f)
		}
	}
	doc, err := personakind.Leaf().Schema.Document()
	if err != nil {
		t.Fatal(err)
	}
	got, was := contribapi.DataTypeFormat(doc), "1.1"
	if got == "" {
		t.Fatal("the leaf schema states no format")
	}
	if v := contribapi.JudgeFormat(was, got); v != contribapi.FormatNewerMinor {
		t.Errorf("leaf schema format %q against the hand-written %q: verdict %d, want a newer minor", got, was, v)
	}
	// The schema carries every field the hand-written one named.
	var schema struct {
		Types map[string]struct {
			Fields map[string]json.RawMessage `json:"fields"`
		} `json:"types"`
	}
	if err := json.Unmarshal(doc, &schema); err != nil {
		t.Fatal(err)
	}
	fields := schema.Types["Persona"].Fields
	for _, f := range []string{"name", "description", "type", "category", "bind_mode", "types",
		"default_gate", "config", "augments", "applies_to"} {
		if _, ok := fields[f]; !ok {
			t.Errorf("leaf schema lacks the field %q: %s", f, doc)
		}
	}
	// A minor reads every 1.1 persona: bind_mode keeps its value set and type
	// stays optional (the task kind requires it; a persona never did).
	var field struct {
		Optional bool     `json:"optional"`
		Enum     []string `json:"enum"`
	}
	if err := json.Unmarshal(fields["bind_mode"], &field); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(field.Enum, "|"); got != "object|global|custom" {
		t.Errorf("bind_mode enum %q, want object|global|custom as in 1.1: %s", got, fields["bind_mode"])
	}
	field.Optional = false
	if err := json.Unmarshal(fields["type"], &field); err != nil {
		t.Fatal(err)
	}
	if !field.Optional {
		t.Errorf("type is required in the leaf schema; 1.1 has it optional: %s", fields["type"])
	}
}
