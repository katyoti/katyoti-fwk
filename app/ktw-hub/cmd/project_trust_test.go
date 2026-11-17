// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	pcapi "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
)

// The hub tells each project's member whether the universe trusts the
// project (projects.yaml trusted): the trusted project's override entries
// report Trusted, the untrusted one's do not; editing projects.yaml and
// reading the membership again flips it, the member whose trust moved named
// for its record to be rebuilt.
func TestAProjectMemberCarriesTheTrustOfItsProject(t *testing.T) {
	prev := projectMembers
	projectMembers = newProjectMemberRegistry()
	t.Cleanup(func() { projectMembers = prev })

	newProject := func(id string) string {
		root := t.TempDir()
		writeProject(t, root, "id: "+id+"\n")
		if err := os.MkdirAll(filepath.Join(root, "modules", "sys-contributions"), 0o755); err != nil {
			t.Fatal(err)
		}
		return root
	}
	shop := newProject("Acme Shop")
	bank := newProject("Acme Bank")
	home := t.TempDir()
	attach := func(trustShop, trustBank bool) {
		t.Helper()
		b := func(v bool) string {
			if v {
				return "true"
			}
			return "false"
		}
		body := "projects:\n" +
			"  - path: '" + shop + "'\n    trusted: " + b(trustShop) + "\n" +
			"  - path: '" + bank + "'\n    trusted: " + b(trustBank) + "\n"
		if err := os.WriteFile(filepath.Join(home, cenv.AttachedProjectsFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	trusted := func(root string) bool { return cenv.ProjectTrusted(home, root) }
	read := func() []string {
		t.Helper()
		_, refused, served := membershipOf(&appcore.UniverseSidecars{}, []string{shop, bank})
		if len(refused) != 0 || len(served) != 2 {
			t.Fatalf("served %+v, refused %v; want both projects", served, refused)
		}
		return projectMembers.keepAll(served, trusted)
	}

	attach(true, false)
	if moved := read(); len(moved) != 0 {
		t.Fatalf("first read moved %v; a new member is told, not moved", moved)
	}
	ctx := context.Background()
	for _, token := range []string{"acme-shop", "acme-bank"} {
		e := pcapi.ObjectEntry{Entry: contribapi.Contribution{Provider: token, Type: "persona", ID: "reviewer-fr"},
			Object: "reviewer-fr", Overrides: "ktw-example:/persona/reviewer"}
		e.Entry.SetLayer("prompt", []byte("Relis."), "text/markdown")
		if err := projectMembers.get(token).Apply(pcapi.EntriesPush{Token: token, Op: pcapi.PushPut, Object: "reviewer-fr", Entry: &e}); err != nil {
			t.Fatal(err)
		}
	}
	trustOf := func(token string) bool {
		t.Helper()
		ps, err := projectMembers.get(token).Providers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range ps {
			if r, ok := p.(contribapi.OverrideEntryReporter); ok {
				if ovs := r.OverrideEntries(); len(ovs) == 1 {
					return ovs[0].Trusted
				}
			}
		}
		t.Fatalf("%s reports no override entry", token)
		return false
	}
	if !trustOf("acme-shop") || trustOf("acme-bank") {
		t.Fatalf("shop trusted %v, bank trusted %v; want the trusted project's alone", trustOf("acme-shop"), trustOf("acme-bank"))
	}

	// projects.yaml edited: the shop untrusted, the bank trusted.
	attach(false, true)
	if moved := read(); len(moved) != 2 || moved[0] != "acme-bank" || moved[1] != "acme-shop" {
		t.Fatalf("moved %v; want both members", moved)
	}
	if trustOf("acme-shop") || !trustOf("acme-bank") {
		t.Fatalf("after the edit shop trusted %v, bank trusted %v; want it flipped", trustOf("acme-shop"), trustOf("acme-bank"))
	}
	ms := projectMembers.takeTrustMoved()
	if len(ms) != 2 || ms[0].Token() != "acme-bank" || ms[1].Token() != "acme-shop" {
		t.Fatalf("taken %d members; want both, to rebuild their records", len(ms))
	}
	if again := projectMembers.takeTrustMoved(); len(again) != 0 {
		t.Fatalf("taken twice: %d", len(again))
	}
	if moved := read(); len(moved) != 0 {
		t.Fatalf("an unchanged projects.yaml moved %v", moved)
	}
}
