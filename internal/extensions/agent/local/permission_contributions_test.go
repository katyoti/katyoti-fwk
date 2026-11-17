// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	agentcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/agent/contrib"
	agentlocal "github.com/katyoti/katyoti-fwk/internal/extensions/agent/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// The permission gate's profiles are `permission-profile` contributions
// . With the universe unreachable (the hub down, the catalog
// cold), the service reads this binary's own: the default resolves, and a profile another
// member publishes (ktw-compose's compose-edit) resolves when it is there.
func TestPermissionProfiles_ResolveFromContributionsWithTheCatalogCold(t *testing.T) {
	defer agentlocal.UniverseUnreachable()()
	compose := loader.NewFSLoader(filepath.Join("..", "..", "..", "..", "app", "ktw-compose", "embed"))
	ctx, _ := cwmapp.New(t,
		agentcontrib.DeclareKinds(""),
		agentcontrib.OwnProfiles(""),
		agentcontrib.ContributeProfiles("ktw-compose", compose, "permission-profiles"),
		agentlocal.PermissionProfilesExtension,
	)
	pp, err := application.UseService[permrules.Profiles](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def := pp.Active("")
	if def == nil || def.Name != "workspace-edit" {
		t.Fatalf("default profile = %+v, want workspace-edit", def)
	}
	if len(def.AllowRules) == 0 {
		t.Error("workspace-edit carries no allow rule")
	}
	ce := pp.Active("compose-edit")
	if ce == nil || ce.Name != "compose-edit" {
		t.Fatalf("compose-edit = %+v, want the profile ktw-compose publishes", ce)
	}
	if ok, _ := ce.AllowMatches("bash", map[string]string{"command": "go test"}); !ok {
		t.Error("compose-edit does not allow bash")
	}
	if plan := pp.Active("plan-mode"); plan == nil || len(plan.DenyRules) == 0 {
		t.Errorf("plan-mode = %+v, want its deny rules", plan)
	}
	list, err := application.UseService[agentlocal.PermissionProfiles](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(list.List()); got != 4 {
		t.Errorf("List = %v, want the 4 contributed profiles", list.List())
	}
}

// The bespoke profile loader is gone and stays gone: profiles are read only
// as contributions, never from a hand-wired folder.
func TestPermissionProfiles_NoBespokeLoader(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	for _, dir := range []string{"app/ktw-agents/cmd", "internal/extensions/agent/local", "internal/extensions/agent/api/permrules"} {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(root, dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, banned := range []string{"PermissionProfilesLoader", "config/Permissions\""} {
				if strings.Contains(string(raw), banned) {
					t.Errorf("%s/%s names %s: permission profiles are contributions", dir, e.Name(), banned)
				}
			}
		}
	}
}
