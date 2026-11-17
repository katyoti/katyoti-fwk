// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
	skillscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/skills/contrib"
	skillcore "github.com/katyoti/katyoti-fwk/internal/extensions/skills/internal/core"
)

func skillMD(name string) string {
	return "---\nname: " + name + "\ndescription: \"projection test skill " + name + "\"\n---\n# " + name + "\n\nBody.\n"
}

// TestExtraDirsIsExecutionOnlyNeverDiscovery is e17's negative parallel-
// registry test. SkillManager is a projection for EXECUTION: besides the
// loader-backed providers it scans `SkillManager.yaml` extra_dirs, because a
// script run needs raw bytes on a path. That second input must never become a
// second DISCOVERY path: a skill that exists only in an extra dir is reachable
// by the manager (its stated purpose) and absent from SkillsFromCatalog over a
// catalog built from the providers — the list every agent prompt and picker
// reads.
func TestExtraDirsIsExecutionOnlyNeverDiscovery(t *testing.T) {
	extra := t.TempDir()
	if err := os.MkdirAll(filepath.Join(extra, "extra-only"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extra, "extra-only", "SKILL.md"), []byte(skillMD("extra-only")), 0o644); err != nil {
		t.Fatal(err)
	}

	packed := loader.NewMapLoader(map[string]string{"skills/packed/SKILL.md": skillMD("packed")})
	app := application.New(application.Config(),
		func(init *application.ApplicationInit) {
			init.AddRegisterPoints("skills.projection_test", ".point", func(app *application.Application) error {
				return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
			})
		},
		contribution.SystemContributionsExtension(),
		skillscontrib.ContributeSkillResources("pack", packed, "skills"))
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)

	// The execution projection: providers + extra dirs.
	mgr, err := skillcore.NewSkillManager(ctx, &skillcore.SkillManagerConfig{ExtraDirs: []string{extra}})
	if err != nil {
		t.Fatalf("NewSkillManager: %v", err)
	}
	if _, err := mgr.GetSkill("user:/skill/extra-only"); err != nil {
		t.Fatalf("the extra-dir skill is not reachable for execution: %v", err)
	}
	if _, err := mgr.GetSkill("pack:/skill/packed"); err != nil {
		t.Fatalf("the provider's skill is not reachable for execution: %v", err)
	}

	// The discovery authority: a catalog built from the providers.
	m, err := manifestlocal.NewBuilder("pack", "1.0.0").Build(ctx)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	var cat mbapi.Catalog = manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{m}, nil)
	listed, err := api.SkillsFromCatalog(ctx, cat)
	if err != nil {
		t.Fatalf("SkillsFromCatalog: %v", err)
	}
	seen := map[string]bool{}
	for _, s := range listed {
		seen[s.Name] = true
	}
	if !seen["packed"] {
		t.Fatalf("the provider's skill is missing from discovery (%v) — the test is not reading the catalog", listed)
	}
	if seen["extra-only"] {
		t.Errorf("a skill present only in an extra dir is DISCOVERABLE — extra_dirs has become a second discovery path")
	}
}
