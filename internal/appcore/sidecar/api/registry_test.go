// SPDX-License-Identifier: Apache-2.0

package api

import (
	"os"
	"path/filepath"
	"testing"
)

// A member is a binary to run, a folder the hub reads, or a remote service on
// the bus: IsFolder names the folder member (no binary, or the generic host
// with a folder that has no scripts), Validate the member that is none of the
// three.
func TestAMemberIsABinaryAFolderOrRemote(t *testing.T) {
	noScripts, withScripts := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(withScripts, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		spec   SidecarSpec
		folder bool
		valid  bool
	}{
		{SidecarSpec{Name: "a", Binary: "a.exe"}, false, true},
		{SidecarSpec{Name: "b", ContentRoot: "/packs/b"}, true, true},
		// The generic host with a folder that has no scripts/: the hub reads it.
		{SidecarSpec{Name: "c", Binary: "C:/bin/ktw-host.exe", ContentRoot: noScripts}, true, true},
		// ... with scripts/: the host's process runs them.
		{SidecarSpec{Name: "c2", Binary: "ktw-host", ContentRoot: withScripts}, false, true},
		// Another binary with a content root reads its folder itself.
		{SidecarSpec{Name: "p", Binary: "polyglot-producer.exe", ContentRoot: noScripts}, false, true},
		{SidecarSpec{Name: "d", Transport: "remote"}, false, true},
		{SidecarSpec{Name: "e"}, false, false},
		{SidecarSpec{Binary: "x.exe"}, false, false},
	}
	for _, c := range cases {
		if got := c.spec.IsFolder(); got != c.folder {
			t.Errorf("%+v: IsFolder = %v, want %v", c.spec, got, c.folder)
		}
		if err := c.spec.Validate(); (err == nil) != c.valid {
			t.Errorf("%+v: Validate = %v, want valid %v", c.spec, err, c.valid)
		}
	}
}
