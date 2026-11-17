// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/available"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/infra"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/registry"
)

// devInstallation lays out, in a root of the test's own, what make dev leaves
// : this test binary - a Go build with no release stamp, as
// make dev's are - copied as the control plane's katyoti and as two programs
// of the dev channel, nothing recorded for them; and one of those two also
// installed from a marketplace (its store row at v1.0.0), as a solution's
// install leaves it.
func devInstallation(t *testing.T) (root string, s *service) {
	t.Helper()
	root = t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	t.Setenv(infra.EnvChannel, seedDeployRoot(t, "v2.0.0"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	exe, err := os.Executable()
	if err != nil {
		t.Skip("no executable path:", err)
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	copyExe(t, exe, filepath.Join(root, "control-plane", "katyoti"+suffix))
	copyExe(t, exe, filepath.Join(root, "bin", "dev", "ktw-agents"+suffix))
	copyExe(t, exe, filepath.Join(root, "bin", "dev", "ktw-code"+suffix))
	stored := filepath.Join(root, "bin", "store", "ktw-agents", "v1.0.0", "ktw-agents"+suffix)
	copyExe(t, exe, stored)
	if err := available.Record(mktapi.Installed{
		Name: "ktw-agents", Kind: "sidecar", Version: "v1.0.0", Source: "marketplace", Marketplace: "acme",
		BinaryPath: stored,
	}); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.New()
	if err != nil {
		t.Fatal(err)
	}
	// The marketplace the store copy came from publishes a newer version: the
	// dev build is still not behind it.
	catalog := filepath.Join(t.TempDir(), "katyoti-marketplace.yaml")
	if err := os.WriteFile(catalog, []byte(`apiVersion: 1
marketplace:
    name: acme
items:
    - name: ktw-agents
      kind: sidecar
      versions:
        - version: v2.0.0
        - version: v1.0.0
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add("acme", catalog, "", ""); err != nil {
		t.Fatal(err)
	}
	return root, &service{reg: reg}
}

func copyExe(t *testing.T, from, to string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// After make dev the control plane is installed - a dev build of a checkout,
// with its commit - never « not installed », and never behind the channel's
// version.
func TestADevInstallationsControlPlaneIsItsDevBuild(t *testing.T) {
	_, s := devInstallation(t)
	v, err := s.InfraVersions(context.Background())
	if err != nil {
		t.Fatalf("InfraVersions: %v", err)
	}
	if !v.DevBuild || v.Installed == "" {
		t.Fatalf("the control plane make dev placed reads %+v; want a dev build with its version", v)
	}
	if v.Current != "v2.0.0" || v.Channel == "" {
		t.Errorf("the channel is still said (what install-infra would place): %+v", v)
	}

	// The channel out of reach does not hide it: it does not come from there.
	t.Setenv(infra.EnvChannel, filepath.Join(t.TempDir(), "gone"))
	v, err = s.InfraVersions(context.Background())
	if err != nil || !v.DevBuild || v.Installed == "" {
		t.Errorf("with no channel: %+v, %v; want the dev build said", v, err)
	}
}

// Each program of the dev channel is one row, shown as its dev build - the one
// a store copy is kept for too - and none is « behind » a marketplace: make
// dev moves them.
func TestADevInstallationsSidecarsAreItsDevBuilds(t *testing.T) {
	root, s := devInstallation(t)
	items, err := s.ListInstalled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]int{}
	byName := map[string]mktapi.Installed{}
	for _, it := range items {
		rows[it.Name]++
		byName[it.Name] = it
	}
	for _, name := range []string{"ktw-agents", "ktw-code"} {
		if rows[name] != 1 {
			t.Errorf("%s has %d rows; want one", name, rows[name])
			continue
		}
		b := byName[name].DevBuild
		if b == nil || b.Version == "" || filepath.Dir(b.Path) != filepath.Join(root, "bin", "dev") {
			t.Errorf("%s is not shown as the dev channel's build: %+v", name, byName[name])
		}
	}
	if byName["ktw-agents"].Version != "v1.0.0" || byName["ktw-agents"].Source != "marketplace" {
		t.Errorf("the store copy kept beside the dev build is lost: %+v", byName["ktw-agents"])
	}
	for _, it := range items {
		if it.Name == "katyoti" && !it.ControlPlane {
			t.Errorf("katyoti is not marked a control-plane program: %+v", it)
		}
	}

	ups, err := s.Outdated(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range ups {
		if u.Name == "ktw-agents" || u.Name == "ktw-code" {
			t.Errorf("a dev build is reported behind %s: %+v", u.Latest, u)
		}
	}
}

// A stable installation - no dev channel, a control plane from the channel -
// reads as before: what install-infra recorded.
func TestAStableInstallationReadsItsRecords(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	t.Setenv(infra.EnvChannel, seedDeployRoot(t, "v2.0.0"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	cp := filepath.Join(root, "control-plane", "katyoti"+suffix)
	if err := os.MkdirAll(filepath.Dir(cp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cp, []byte("a channel's program"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := available.Record(mktapi.Installed{Name: "katyoti", Version: "v1.0.0", Source: SourceInfraChannel, BinaryPath: cp}); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.New()
	if err != nil {
		t.Fatal(err)
	}
	v, err := (&service{reg: reg}).InfraVersions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v.DevBuild || v.Installed != "v1.0.0" || v.Current != "v2.0.0" {
		t.Errorf("a channel's control plane reads %+v; want v1.0.0 installed, v2.0.0 current, no dev build", v)
	}
}

// The control plane make dev placed is never behind the infra channel - only
// an explicit update --infra replaces it - and that update says it replaced
// the dev build, even over an older install-infra record at the channel's
// version.
func TestADevInstallationsControlPlaneIsNotOutdated(t *testing.T) {
	root, s := devInstallation(t)
	ups, err := s.InfraOutdated(context.Background())
	if err != nil {
		t.Fatalf("InfraOutdated: %v", err)
	}
	if len(ups) != 0 {
		t.Errorf("a dev control plane is reported behind the channel: %+v", ups)
	}

	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	cp := filepath.Join(root, "control-plane", "katyoti"+suffix)
	if err := available.Record(mktapi.Installed{Name: "katyoti", Version: "v2.0.0", Source: SourceInfraChannel, BinaryPath: cp}); err != nil {
		t.Fatal(err)
	}
	dev, _ := controlPlaneDevBuild()
	done, err := (&infraOps{s}).SelfUpdate(context.Background())
	if err != nil {
		t.Fatalf("SelfUpdate: %v", err)
	}
	if len(done) != 1 || done[0].Name != "katyoti" || done[0].Installed != dev || done[0].Latest != "v2.0.0" {
		t.Fatalf("update --infra on a dev build moved %+v; want katyoti from the dev build %q to v2.0.0", done, dev)
	}
	if _, still := controlPlaneDevBuild(); still {
		t.Errorf("the dev build is still in place after update --infra")
	}
}
