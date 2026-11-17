// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	stdreflect "reflect"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	guidelint "github.com/katyoti/katyoti-fwk/internal/extensions/guide/lint"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	contentlint "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/lint"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// checkServices are the services a shell recipe may name as its check: the
// provider and slot of the URI, the service definition that says a method is
// read-only, and the Go interface whose method signature gives its output.
var checkServices = map[string]struct {
	def   meta.ServiceDef
	iface stdreflect.Type
}{
	"ktw-univers:universe-checks": {orchapi.UniverseChecksServiceDef, stdreflect.TypeOf((*orchapi.UniverseChecks)(nil)).Elem()},
	"ktw-agents:inbox-checks":     {inboxapi.InboxChecksServiceDef, stdreflect.TypeOf((*inboxapi.InboxChecks)(nil)).Elem()},
}

// describe answers what ActionService.Describe would, from the declarations
// themselves: the method's ReadOnly flag and the JSON Schema of its real
// output type. A URI no check service publishes cannot be described.
func describe(_ context.Context, uri string) (guidelint.Action, error) {
	u, err := mbapi.ParseToolURI(uri)
	if err != nil {
		return guidelint.Action{}, err
	}
	svc, ok := checkServices[u.Provider+":"+u.SlotType]
	if !ok {
		return guidelint.Action{}, fmt.Errorf("no check service %s:%s", u.Provider, u.SlotType)
	}
	for name, def := range svc.def.Methods {
		if ktwreflect.ToKebab(name) != u.Method {
			continue
		}
		m, ok := svc.iface.MethodByName(name)
		if !ok || m.Type.NumOut() != 2 {
			return guidelint.Action{}, fmt.Errorf("%s: declared but not on the interface", uri)
		}
		schema, err := ktwreflect.JSONSchemaForType(m.Type.Out(0))
		if err != nil {
			return guidelint.Action{}, err
		}
		return guidelint.Action{ReadOnly: def.ReadOnly, OutputSchema: schema}, nil
	}
	return guidelint.Action{}, fmt.Errorf("%s: no such method", uri)
}

// recipePaths are the texts the guide-recipe kind declares translatable.
func recipePaths(t *testing.T) []contribapi.TranslatablePath {
	t.Helper()
	k, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	return k.Translatable(guideapi.Kind)
}

func shellRecipes(t *testing.T) map[string]guideapi.Recipe {
	t.Helper()
	out := map[string]guideapi.Recipe{}
	err := fs.WalkDir(embeddedFS, "embed/guide", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(p) != ".yaml" || strings.HasSuffix(p, ".i18n.yaml") {
			return err
		}
		raw, err := fs.ReadFile(embeddedFS, p)
		if err != nil {
			return err
		}
		id := strings.TrimSuffix(path.Base(p), ".yaml")
		if res := guidelint.LintRecipeBytes(raw); len(res.Findings) > 0 {
			t.Errorf("%s: %s %+v", id, res.Status, res.Findings)
		}
		// Multilingual from the start (OD-015): every shell recipe ships its
		// translations, complete — no step falls back to English.
		tr, err := fs.ReadFile(embeddedFS, strings.TrimSuffix(p, ".yaml")+".i18n.yaml")
		if err != nil {
			t.Errorf("%s: no translations beside it: %v", id, err)
		} else if fs := contentlint.LintTranslationsFiles(recipePaths(t), raw, tr, ""); len(fs) > 0 {
			t.Errorf("%s translations: %+v", id, fs)
		}
		r, err := guideapi.ParseRecipe(raw)
		if err != nil {
			return err
		}
		out[id] = r
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestWelcomeTourRecipe is the G-001 probe of modules/guide (its machine
// half; a fresh install walked by a person is the other). ktw-hub — the
// member of every universe view, admin included, so there before any user
// universe and in every universe an agent chats in — carries the
// welcome tour and the shell recipes; every one lints clean with no warning,
// every `requires` names one of them, the welcome tour ends in checks, and
// every check any of them names is a read-only method answering ok and
// detail, as declared by the service that owns it.
func TestWelcomeTourRecipe(t *testing.T) {
	recipes := shellRecipes(t)
	welcome, ok := recipes["welcome"]
	if !ok {
		t.Fatalf("no welcome tour among %d shell recipes", len(recipes))
	}
	if welcome.IsTour() || len(welcome.Checks()) < 3 {
		t.Errorf("the welcome tour must prove its first success (universe, project, inbox): checks %v", welcome.Checks())
	}
	if len(recipes) < 11 {
		t.Errorf("%d shell recipes, want the welcome tour and the ten first recipes", len(recipes))
	}
	for id, r := range recipes {
		for _, req := range r.Requires {
			if _, ok := recipes[req]; !ok {
				t.Errorf("%s requires %q, which is not a shell recipe", id, req)
			}
		}
		if res := guidelint.LintChecks(context.Background(), r, describe); res.Status != "pass" {
			t.Errorf("%s: checks %+v", id, res.Findings)
		}
	}
}

// The describer is strict: a check naming a method that is not read-only, or
// one no check service publishes, is refused — so the probe above cannot pass
// by describing everything as fine.
func TestWelcomeTourRecipe_DescriberRefuses(t *testing.T) {
	mutating := guideapi.Recipe{Name: "x", Steps: []guideapi.Step{{Say: "x", Check: "ktw-univers:orchestrator/delete-universe"}}}
	if res := guidelint.LintChecks(context.Background(), mutating, describe); res.Status == "pass" {
		t.Error("a check on the orchestrator (not a check service) passed")
	}
	unknown := guideapi.Recipe{Name: "x", Steps: []guideapi.Step{{Say: "x", Check: "ktw-univers:universe-checks/nope"}}}
	if res := guidelint.LintChecks(context.Background(), unknown, describe); res.Status == "pass" {
		t.Error("an unknown check method passed")
	}
}

// TestTheUsageTourAsksNoProfile: the usage tour - a solution's first guide,
// ArchiFactory's among them - asks nothing about the person's profile: the
// installation already set it. It opens the menu for the
// display level and the guides, and opens no profile form.
func TestTheUsageTourAsksNoProfile(t *testing.T) {
	tour, ok := shellRecipes(t)["use-katyoti"]
	if !ok {
		t.Fatal("no use-katyoti tour among the shell recipes")
	}
	for _, s := range tour.Steps {
		if s.ID == "profile" || strings.Contains(s.Say, "{control:profile-menu.profile}") || (s.Target != nil && s.Target.Guide == "profile-menu.profile") {
			t.Errorf("step %q asks for the profile: %q", s.ID, s.Say)
		}
	}
}
