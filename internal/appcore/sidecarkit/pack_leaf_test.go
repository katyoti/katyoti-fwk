// SPDX-License-Identifier: Apache-2.0

package sidecarkit

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

type fakeSkillBody struct {
	Name string `json:"name"`
}

// fakeSkillLeaf declares the system kind `skill` as its pack would, its
// description changed.
func fakeSkillLeaf() leaf.Declaration {
	return leaf.Declaration{
		Kind:   "skill",
		YAML:   []byte("name: skill\ndescription: A test leaf.\nsource:\n  folder: skills\n  match: [\"*.md\"]\n"),
		Schema: manifestcontrib.GoTypeOf[fakeSkillBody]("skill", "7.3.0", "Skill", "A test leaf schema."),
	}
}

func testProvidersPoint(init *application.ApplicationInit) {
	init.AddRegisterPoints("test", ".providers-point", func(app *application.Application) error {
		return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
	})
}

// published runs a sidecar pack of fsys with leaves and counts each URI the
// catalog's providers enumerate.
func published(t *testing.T, fsys fstest.MapFS, remap map[string]string, leaves []leaf.Declaration) map[string]int {
	t.Helper()
	embedded := loader.NewEmbedFSLoader(fsys, ".")
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		testProvidersPoint,
		contribution.SystemContributionsExtension(),
		packExtension("ktw-test", embedded, embedded, "embed", remap, leaves),
	)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			count[c.ResolvedURI()]++
		}
	}
	return count
}

// A leaf kind whose folder the embedded pack holds is declared by the binary,
// once; a pack without that folder, or with the kind switched off, declares
// none (the system set never carries a pack's kind).
func TestPackExtension_DeclaresTheLeafKindsItHolds(t *testing.T) {
	withSkills := fstest.MapFS{
		"embed/skills/demo/SKILL.md": {Data: []byte("---\nname: demo\ndescription: A demo skill.\n---\nBody.\n")},
	}
	noSkills := fstest.MapFS{
		"embed/references/demo.md": {Data: []byte("# Demo\n")},
	}
	leaves := []leaf.Declaration{fakeSkillLeaf()}
	for _, tc := range []struct {
		name  string
		fsys  fstest.MapFS
		remap map[string]string
		leaf  bool
	}{
		{"held", withSkills, nil, true},
		{"not held", noSkills, nil, false},
		{"switched off", withSkills, map[string]string{"skill": ""}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := published(t, tc.fsys, tc.remap, leaves)
			want := map[string]int{
				"ktw-test:/contribution-type/skill":  0,
				"ktw-test:/data-type/skill":          0,
				"system:/contribution-type/skill":    0,
				"system:/contribution-type/task":     0,
				"system:/contribution-type/workflow": 1,
			}
			if tc.leaf {
				want["ktw-test:/contribution-type/skill"] = 1
				want["ktw-test:/data-type/skill"] = 1
			}
			for uri, n := range want {
				if count[uri] != n {
					t.Errorf("%s published %d times, want %d", uri, count[uri], n)
				}
			}
		})
	}
}
