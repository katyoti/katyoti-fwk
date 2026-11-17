// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// fakeOrch answers GetUniverse and ListProjects from maps; any other call
// panics (nil embedded interface), so the checks prove they only read.
type fakeOrch struct {
	api.OrchestratorService
	universes map[string]api.UniverseInfo
	projects  map[string][]string
}

func (f fakeOrch) GetUniverse(_ context.Context, in api.GetUniverseInput) (api.UniverseInfo, error) {
	u, ok := f.universes[in.Name]
	if !ok {
		return api.UniverseInfo{}, errors.New("not registered")
	}
	return u, nil
}

func (f fakeOrch) ListProjects(_ context.Context, in api.ListProjectsInput) (api.ListProjectsOutput, error) {
	return api.ListProjectsOutput{Projects: f.projects[in.Universe]}, nil
}

func TestUniverseChecks(t *testing.T) {
	c := ChecksOver(fakeOrch{
		universes: map[string]api.UniverseInfo{
			"admin": {Summary: api.UniverseSummary{Name: "admin", Protected: true}},
			"mine":  {Summary: api.UniverseSummary{Name: "mine", DisplayName: "Mine"}, Sidecars: []api.SidecarSpec{{Name: "ktw-agents"}}},
			"wired": {Summary: api.UniverseSummary{Name: "wired"}, Sidecars: []api.SidecarSpec{{Name: "ktw-agents"}, {Name: "ktw-example"}}},
		},
		projects: map[string][]string{"wired": {"D:/work/repo"}},
	})
	ctx := context.Background()
	in := func(u string, with map[string]string) guideapi.CheckInput {
		return guideapi.CheckInput{Universe: u, With: with}
	}
	cases := []struct {
		name string
		run  func() (guideapi.CheckResult, error)
		ok   bool
	}{
		{"ready: none on screen", func() (guideapi.CheckResult, error) { return c.UniverseReady(ctx, in("", nil)) }, false},
		{"ready: unknown", func() (guideapi.CheckResult, error) { return c.UniverseReady(ctx, in("nope", nil)) }, false},
		{"ready: admin is not yours", func() (guideapi.CheckResult, error) { return c.UniverseReady(ctx, in("admin", nil)) }, false},
		{"ready: user universe", func() (guideapi.CheckResult, error) { return c.UniverseReady(ctx, in("mine", nil)) }, true},
		{"project: none", func() (guideapi.CheckResult, error) { return c.ProjectAttached(ctx, in("mine", nil)) }, false},
		{"project: one", func() (guideapi.CheckResult, error) { return c.ProjectAttached(ctx, in("wired", nil)) }, true},
		{"capability: only ktw-agents", func() (guideapi.CheckResult, error) { return c.CapabilityWired(ctx, in("mine", nil)) }, false},
		{"capability: any beyond ktw-agents", func() (guideapi.CheckResult, error) { return c.CapabilityWired(ctx, in("wired", nil)) }, true},
		{"capability: the named one", func() (guideapi.CheckResult, error) {
			return c.CapabilityWired(ctx, in("wired", map[string]string{"sidecar": "ktw-example"}))
		}, true},
		{"capability: named but absent", func() (guideapi.CheckResult, error) {
			return c.CapabilityWired(ctx, in("wired", map[string]string{"sidecar": "ktw-docs"}))
		}, false},
	}
	for _, tc := range cases {
		r, err := tc.run()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if r.OK != tc.ok || r.Detail == "" {
			t.Errorf("%s: got %+v, want ok=%v with a detail", tc.name, r, tc.ok)
		}
	}
}

// TestRepositoriesHaveRemote: a universe whose home or project sits in a git
// repository with no remote fails the check with a warning naming the
// repository; once every repository has a remote, it passes; a universe in
// no repository passes too.
func TestRepositoriesHaveRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	ctx := context.Background()
	gitIn := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	home := t.TempDir()
	project := t.TempDir()
	gitIn(project, "init", "--quiet")
	c := ChecksOver(fakeOrch{
		universes: map[string]api.UniverseInfo{"mine": {Summary: api.UniverseSummary{Name: "mine", HomePath: home}}},
		projects:  map[string][]string{"mine": {project}},
	})
	in := guideapi.CheckInput{Universe: "mine"}

	r, err := c.RepositoriesHaveRemote(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if r.OK || !strings.Contains(r.Detail, "not backed up") || r.DetailMsg == nil || r.DetailMsg.Key != "checks.noRemote" {
		t.Fatalf("project with no remote: got %+v, want a not-backed-up warning", r)
	}

	gitIn(project, "remote", "add", "backup", "https://example.invalid/repo.git")
	if r, _ = c.RepositoriesHaveRemote(ctx, in); !r.OK {
		t.Fatalf("project with a remote: got %+v, want ok", r)
	}

	bare := ChecksOver(fakeOrch{universes: map[string]api.UniverseInfo{"loose": {Summary: api.UniverseSummary{Name: "loose", HomePath: home}}}})
	if r, _ = bare.RepositoriesHaveRemote(ctx, guideapi.CheckInput{Universe: "loose"}); !r.OK {
		t.Fatalf("no repository: got %+v, want ok", r)
	}
	if r, _ = c.RepositoriesHaveRemote(ctx, guideapi.CheckInput{}); r.OK {
		t.Fatal("no universe on screen: want a failure")
	}
}
