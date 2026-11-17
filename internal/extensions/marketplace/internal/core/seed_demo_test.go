// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
)

// A seed its publisher marks as demo data (build-seed --demo) is listed and
// shown as one, so the Marketplace draws its « Démo » badge; a seed published
// without the mark is not.
func TestASeedPublishedAsDemoIsListedAsDemo(t *testing.T) {
	s := solutionFixture(t, "")
	mkts, err := s.reg.List()
	if err != nil || len(mkts) != 1 {
		t.Fatalf("marketplaces %+v, %v", mkts, err)
	}
	out := filepath.Dir(mkts[0].URL)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Projet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, demo := range map[string]bool{"showcase": true, "starter": false} {
		if _, err := authoring.New().BuildSeed(project, "", name, "1", "", "", nil, out, 0, "", demo); err != nil {
			t.Fatal(err)
		}
	}

	seeds, err := s.ListSeeds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, sd := range seeds {
		got[sd.Name] = sd.Demo
	}
	if len(got) != 2 || !got["showcase"] || got["starter"] {
		t.Errorf("listed seeds %+v; want showcase demo, starter not", seeds)
	}
	for name, want := range map[string]bool{"showcase": true, "starter": false} {
		shown, err := s.ShowSeed(context.Background(), mktapi.ShowSeedInput{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		if shown.Demo != want {
			t.Errorf("show %s: demo %v; want %v", name, shown.Demo, want)
		}
	}
}
