// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// Every attached project's contributions folder is a folder member under its
// own provider token, held by the project members and served: two projects,
// two tokens, the skills of both recorded. Two projects under one token: the
// first attached is served, the second refused with a finding naming both.
func TestEveryAttachedProjectIsAFolderMember(t *testing.T) {
	prev := projectMembers
	projectMembers = newProjectMemberRegistry()
	t.Cleanup(func() { projectMembers = prev })

	newProject := func(id, skill string) string {
		root := t.TempDir()
		writeProject(t, root, "id: "+id+"\n")
		skills := filepath.Join(root, "modules", "sys-contributions", "skills")
		if err := os.MkdirAll(skills, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: " + skill + "\ndescription: A skill\n---\nDo it.\n"
		if err := os.WriteFile(filepath.Join(skills, skill+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return root
	}
	shop := newProject("Acme Shop", "greet")
	bank := newProject("Acme Bank", "count")

	reg, refused, served := membershipOf(&appcore.UniverseSidecars{}, []string{shop, bank})
	if len(refused) != 0 {
		t.Fatalf("refused %v, want none", refused)
	}
	if len(reg.Sidecars) != 2 || reg.Sidecars[0].Name != "acme-shop" || reg.Sidecars[1].Name != "acme-bank" {
		t.Fatalf("members = %+v; want acme-shop and acme-bank", reg.Sidecars)
	}
	projectMembers.keepAll(served, nil)
	if got := projectMembers.tokens(); len(got) != 2 || got[0] != "acme-bank" || got[1] != "acme-shop" {
		t.Fatalf("held tokens = %v; want both projects'", got)
	}

	cache := manifestlocal.NewCache(t.TempDir())
	saveHubRecord(t, cache, t.TempDir())
	if _, probed, n := hydrateCache(cache, reg.Sidecars, io.Discard); probed != 2 || n != 0 {
		t.Fatalf("hydrate: probed=%d errs=%d, want both folders read", probed, n)
	}
	for _, want := range []struct{ token, skill string }{{"acme-shop", "greet"}, {"acme-bank", "count"}} {
		var m appcore.SidecarSpec
		for _, s := range reg.Sidecars {
			if s.Name == want.token {
				m = s
			}
		}
		rec, _, err := cache.Load(m.Name, recordSource(m))
		if err != nil || rec == nil {
			t.Fatalf("no record of %s: %v", want.token, err)
		}
		if len(rec.Contributions) != 1 || rec.Contributions[0].Provider != want.token || rec.Contributions[0].ID != want.skill {
			t.Fatalf("record of %s = %+v, want its skill %s", want.token, rec.Contributions, want.skill)
		}
	}

	// The same token twice: the first attached wins, the second is refused.
	writeProject(t, bank, "id: Acme Bank\ncapabilities:\n  contributions:\n    provider: acme-shop\n")
	reg, refused, served = membershipOf(&appcore.UniverseSidecars{}, []string{shop, bank})
	if len(reg.Sidecars) != 1 || reg.Sidecars[0].Name != "acme-shop" || len(served) != 1 {
		t.Fatalf("members = %+v, tokens %v; want acme-shop alone", reg.Sidecars, served)
	}
	why := refused["acme-shop"]
	if !strings.Contains(why, shop) || !strings.Contains(why, bank) {
		t.Fatalf("finding %q; want it naming both projects", why)
	}
	projectMembers.keepAll(served, nil)
	if got := projectMembers.tokens(); len(got) != 1 || got[0] != "acme-shop" {
		t.Fatalf("held tokens = %v; want the dropped project's member released", got)
	}
}
