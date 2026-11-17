// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/memberversion"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// A `kind: release` exists to be REPRODUCED. Its member versions therefore have
// to come from the versioned store, whose shape is `<…>/store/<name>/<version>`.
//
// This used to take the parent folder name blindly, so a dev binary at
// `…/bin/ktw-notify.exe` derived the "version" **bin** — nonsense that looked
// pinned. The release published cleanly and then failed on the CONSUMER's
// machine at bootstrap with `version "bin" not found`: the worst place to learn
// it, for an artifact whose whole purpose is reproducibility.
//
// Every fixture is an OS-native absolute path built with filepath.Join from a
// temporary root: a Windows literal (`C:\store\…`) is not absolute on Linux and
// its backslashes are not separators there, so it would pin nothing and the
// negative cases would pass by chance.
func TestDeriveMemberVersion_OnlyPinsFromTheStore(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name string
		spec orchapi.SidecarSpec
		want string
	}{
		{
			name: "store binary pins its version",
			spec: orchapi.SidecarSpec{Name: "ktw-notify", Binary: filepath.Join(root, "store", "ktw-notify", "v0.1.0", exeName("ktw-notify"))},
			want: "v0.1.0",
		},
		{
			name: "dev binary is NOT pinnable",
			spec: orchapi.SidecarSpec{Name: "ktw-notify", Binary: filepath.Join(root, "bin", exeName("ktw-notify"))},
			want: "",
		},
		{
			// What a foreign-OS literal is on this host: a binary path that is
			// not absolute here pins nothing, even with a store shape.
			name: "non-absolute store binary is NOT pinnable",
			spec: orchapi.SidecarSpec{Name: "ktw-notify", Binary: filepath.Join("store", "ktw-notify", "v0.1.0", exeName("ktw-notify"))},
			want: "",
		},
		{
			name: "content folder in the store pins its version",
			spec: orchapi.SidecarSpec{
				Name: "acme-playbooks",
				Args: []string{"--folder", filepath.Join(root, "store-content", "acme-playbooks", "v1.0.0"), "sc", "ktsp", "nats"},
			},
			want: "v1.0.0",
		},
		{
			name: "content folder OUTSIDE the store is NOT pinnable",
			spec: orchapi.SidecarSpec{
				Name: "acme-playbooks",
				Args: []string{"--folder", filepath.Join(root, "work", "acme-playbooks"), "sc", "ktsp", "nats"},
			},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := memberversion.Derive(tc.spec); got != tc.want {
				t.Errorf("DeriveMemberVersion = %q, want %q", got, tc.want)
			}
		})
	}
}

// And an unpinnable member must REFUSE the snapshot rather than publish a
// release nobody can reproduce. Refusing costs the publisher one message;
// publishing costs whoever tries to use it a debugging session on a broken
// artifact they did not build.
func TestAddRelease_RefusesAnUnreproducibleMember(t *testing.T) {
	// A dev-binary member derives no version, so the snapshot must refuse.
	if v := memberversion.Derive(orchapi.SidecarSpec{
		Name: "ktw-notify", Binary: filepath.Join(t.TempDir(), "bin", exeName("ktw-notify")),
	}); v != "" {
		t.Fatalf("precondition: dev binary derived version %q, want none", v)
	}
	// The refusal itself is exercised end-to-end by the snapshot-universe CLI;
	// here we pin the MESSAGE contract the CLI surfaces, so a future refactor
	// cannot quietly turn the refusal into a silent pin.
	const want = "not the versioned store"
	if !strings.Contains(unreproducibleHint, want) {
		t.Errorf("the refusal hint %q no longer explains WHY the member is unpinnable", unreproducibleHint)
	}
}

// exeName is a binary's file name on this host.
func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
