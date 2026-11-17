// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/prereqs"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// through the « Ready to start » check: an installed sidecar
// whose files were deleted by hand is forgotten from sidecars.yaml and never
// config-checked (no fork/exec error); the universe still wired to it keeps
// the member and names it not installed; a member pinned to a version that
// is gone while another version is installed is rewired to it.
func TestTheReadyCheckForgetsWhatIsGoneAndRewiresTheUniverse(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	t.Setenv("KTW_KATYOTI_ROOT", root)
	t.Setenv("KTW_WORKSPACE", filepath.Join(base, "no-workspace"))
	// The installed items are the registry's and the programs a scan finds on
	// PATH: the machine's PATH (the dev installation's ktw-sync) would replace
	// the item that is gone. The test's PATH holds nothing.
	t.Setenv("PATH", filepath.Join(base, "empty-path"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	exe := func(name, version string) string {
		return filepath.Join(root, "bin", "store", name, version, name+".exe")
	}
	rc5 := exe("ktw-methods", "v2026.10-rc5")
	if err := os.MkdirAll(filepath.Dir(rc5), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rc5, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	sidecars := filepath.Join(root, "sidecars.yaml")
	if err := orchcatalog.SaveSidecarsYaml(sidecars, &orchcatalog.SidecarsYaml{Version: 1, Sidecars: []orchcatalog.SidecarsYamlEntry{
		{Name: "ktw-methods", BinaryPath: rc5, Version: "v2026.10-rc5", Source: "marketplace", Kind: "sidecar"},
		{Name: "ktw-sync", BinaryPath: exe("ktw-sync", "v0.2.0-test.5"), Version: "v0.2.0-test.5", Source: "marketplace", Kind: "sidecar"},
	}}); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "universes", "archifactory")
	if err := orchcatalog.SaveManifest(home, &orchcatalog.Manifest{Version: 1, Name: "archifactory", Sidecars: []api.SidecarSpec{
		{Name: "ktw-methods", Transport: "nats", Binary: exe("ktw-methods", "v0.2.0-test.5"), Args: []string{"sc", "ktsp", "nats"}},
		{Name: "ktw-sync", Transport: "nats", Binary: exe("ktw-sync", "v0.2.0-test.5"), Args: []string{"sc", "ktsp", "nats"}},
	}}); err != nil {
		t.Fatal(err)
	}
	regPath, err := orchcatalog.DefaultRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := orchcatalog.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(orchcatalog.RegistryEntry{Name: "archifactory", HomePath: home}); err != nil {
		t.Fatal(err)
	}

	s := &service{prereqs: &prereqs.Engine{Runner: noInstall{}, Platform: runtime.GOOS}}
	plan, err := s.PlanPrereqs(context.Background(), mktapi.PrereqsPlanInput{Universes: []string{"archifactory"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(plan.NotInstalled, ",") != "ktw-sync" {
		t.Fatalf("not installed %v, want ktw-sync", plan.NotInstalled)
	}
	if strings.Join(plan.Rewired, ",") != "archifactory: ktw-methods" {
		t.Fatalf("rewired %v, want archifactory: ktw-methods", plan.Rewired)
	}
	// ktw-methods' check (a placeholder file here) may fail: it is checked on
	// its new pin. ktw-sync, whose files are gone, never is.
	for _, u := range plan.Unchecked {
		if strings.Contains(u, "ktw-sync") || strings.Contains(u, "v0.2.0-test.5") {
			t.Fatalf("the vanished ktw-sync was checked: %v", plan.Unchecked)
		}
	}
	sy, err := orchcatalog.LoadSidecarsYaml(sidecars)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(sidecars)
	if len(sy.Sidecars) != 1 || strings.Contains(string(raw), "ktw-sync") {
		t.Fatalf("sidecars.yaml still lists ktw-sync:\n%s", raw)
	}
	m, err := orchcatalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sidecars) != 2 || m.Sidecars[0].Binary != rc5 {
		t.Fatalf("universe members %+v: want both kept, ktw-methods on rc5", m.Sidecars)
	}
}
