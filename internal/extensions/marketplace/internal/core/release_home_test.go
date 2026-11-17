// SPDX-License-Identifier: Apache-2.0

package core

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A release bootstrapped with no universe named must land in a NEW universe —
// never in the workspace of the PROCESS serving the call.
//
// This method is served by the ADMIN universe's hub, so "the process workspace"
// is admin. An empty home made bootstrap rewrite admin's own Univers.yaml with
// the release's members: it deleted ktw-univers — the sidecar answering the
// request — and the call then hung forever with the control plane destroyed.
// Found by a GUI click-through, which is exactly the class of bug a unit test
// on either half would have missed.
func TestDefaultReleaseHome_IsNotTheProcessWorkspace(t *testing.T) {
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	got := defaultReleaseHome("demo-release")

	want := filepath.Join(application.Env().GlobalHomeDir(), "universes", "demo-release")
	if got != want {
		t.Errorf("defaultReleaseHome = %q, want %q", got, want)
	}
	if got == application.Env().WorkspaceDir() {
		t.Error("the default IS the process workspace — a hub would rewrite its own universe")
	}
	// It must be under the universes root, which is what makes it a NEW universe
	// rather than whatever folder the server happens to be standing in.
	if !strings.Contains(filepath.ToSlash(got), "/universes/") {
		t.Errorf("%q is not under the global universes root", got)
	}
}

// A universe is named by a registered name, a new name or a folder: a
// registered name is its home, a new name a new universe under the global
// universes root, a folder itself.
func TestAUniverseIsNamedOrAFolder(t *testing.T) {
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	elsewhere := filepath.Join(t.TempDir(), "my-demo")
	regPath, err := orchcatalog.DefaultRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := orchcatalog.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(orchcatalog.RegistryEntry{Name: "known", HomePath: elsewhere}); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(t.TempDir(), "here")
	for target, want := range map[string]string{
		"known":   elsewhere,
		"acme":    filepath.Join(application.Env().GlobalHomeDir(), "universes", "acme"),
		folder:    folder,
		"./local": "./local",
		"":        "",
	} {
		if got := universeTarget(target); got != want {
			t.Errorf("universeTarget(%q) = %q, want %q", target, got, want)
		}
	}
}
