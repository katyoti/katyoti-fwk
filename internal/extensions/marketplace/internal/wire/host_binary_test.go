// SPDX-License-Identifier: Apache-2.0

package wire

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A wired content member must carry a path the hub can STAT.
//
// The hub stats spec.Binary on its spawn path before ever exec'ing it, so the
// bare name this used to emit failed with
//
//	cache check: stat binary: GetFileAttributesEx ktw-host: ...
//
// and the member never spawned — while install, wire, configure and doctor all
// reported success, because only the hub reads spec.Binary. That is why nothing
// caught it: the whole `content` feature was dead at runtime.
//
// The test installs its own ktw-host in a root of its own: it never depends
// on the person's installed-items registry.
func TestBuildSpec_ContentHostIsStattable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	hostName := "ktw-host"
	if runtime.GOOS == "windows" {
		hostName += ".exe"
	}
	host := filepath.Join(root, "bin", "store", "ktw-host", "v1", hostName)
	if err := os.MkdirAll(filepath.Dir(host), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(host, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	reg, err := catalog.DefaultSidecarsPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveSidecarsYaml(reg, &catalog.SidecarsYaml{Version: 1, Sidecars: []catalog.SidecarsYamlEntry{
		{Name: "ktw-host", BinaryPath: host, Version: "v1", Source: "marketplace", Kind: "sidecar"},
	}}); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	// Scripts need the host's process: such a content item keeps the host.
	if err := os.Mkdir(filepath.Join(folder, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := buildSpec(mktapi.Installed{
		Name:       "acme-playbooks",
		Kind:       "content",
		FolderPath: folder,
	})

	if spec.Binary == "" {
		t.Fatal("content member has no Binary")
	}
	// A bare command name is the exact regression: exec.Command would resolve it,
	// os.Stat does not, and the hub uses os.Stat.
	if !filepath.IsAbs(spec.Binary) {
		t.Fatalf("Binary = %q — not absolute, so the hub's pre-exec stat fails", spec.Binary)
	}
	if _, err := os.Stat(spec.Binary); err != nil {
		t.Fatalf("Binary = %q — does not stat: %v", spec.Binary, err)
	}
	base := filepath.Base(spec.Binary)
	want := "ktw-host"
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if base != want {
		t.Errorf("Binary basename = %q, want %q", base, want)
	}
	if spec.Binary != host {
		t.Errorf("Binary = %q, want the installed ktw-host %q", spec.Binary, host)
	}
}

// A compiled sidecar keeps its version-pinned store path untouched.
func TestBuildSpec_SidecarKeepsStorePath(t *testing.T) {
	const pinned = `/store/ktw-notify/v1.2.3/ktw-notify`
	spec := buildSpec(mktapi.Installed{
		Name:       "ktw-notify",
		Kind:       "sidecar",
		BinaryPath: pinned,
	})
	if spec.Binary != pinned {
		t.Errorf("Binary = %q, want the pinned store path %q", spec.Binary, pinned)
	}
}

// A content item without scripts/ is a FOLDER member: its content root and no
// binary — the hub reads it, nothing is spawned.
func TestBuildSpec_ContentWithoutScriptsIsAFolderMember(t *testing.T) {
	folder := t.TempDir()
	spec := buildSpec(mktapi.Installed{Name: "acme-notes", Kind: "content", FolderPath: folder})
	if spec.Binary != "" || len(spec.Args) != 0 || spec.ContentRoot != folder {
		t.Fatalf("spec = %+v, want the content root and no binary", spec)
	}
}
