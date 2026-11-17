// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// « OUVRIR UN DOSSIER » SWITCHES THE SERVED PROJECT: a
// universe serves one project, so opening another detaches the first and
// attaches it; opening the one served changes nothing; a project the attach
// refuses (served by another universe) is refused and the first stays served.
func TestSwitchProjectServesTheOtherAndKeepsTheFirstOnARefusal(t *testing.T) {
	root := projectTestRoot(t)
	seedUniverseFor(t, root, "af")
	seedUniverseFor(t, root, "other")
	a, b, c := filepath.Join(root, "erp-acme"), filepath.Join(root, "cosmos"), filepath.Join(root, "taken")
	for _, d := range []string{a, b, c} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	s := &Service{}
	if _, err := s.AttachProject(ctx, api.AttachProjectInput{Universe: "af", ProjectDir: a}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AttachProject(ctx, api.AttachProjectInput{Universe: "other", ProjectDir: c}); err != nil {
		t.Fatal(err)
	}

	out, err := s.SwitchProject(ctx, api.SwitchProjectInput{Universe: "af", ProjectDir: b})
	if err != nil || !out.Changed || !slices.Equal(out.Left, []string{a}) {
		t.Fatalf("switch to %s: %+v %v; want changed, leaving %s", b, out, err, a)
	}
	if got := s.loadProjects("af").Projects; !slices.Equal(got, []string{b}) {
		t.Errorf("af serves %v; want only %s", got, b)
	}
	if _, err := os.Stat(filepath.Join(application.EnvOf(a).HomeDir(), "project.yaml")); !os.IsNotExist(err) {
		t.Errorf("the project left still points at its universe (%v)", err)
	}

	if out, err := s.SwitchProject(ctx, api.SwitchProjectInput{Universe: "af", ProjectDir: b}); err != nil || out.Changed {
		t.Errorf("switch to the project served: %+v %v; want nothing changed", out, err)
	}

	if _, err := s.SwitchProject(ctx, api.SwitchProjectInput{Universe: "af", ProjectDir: c}); err == nil {
		t.Fatalf("switched to %s, which universe other still serves", c)
	}
	if got := s.loadProjects("af").Projects; !slices.Equal(got, []string{b}) {
		t.Errorf("after a refused switch af serves %v; want %s still", got, b)
	}
}
