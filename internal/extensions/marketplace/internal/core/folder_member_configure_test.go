// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// A content item installed as a folder member (no scripts/) runs no process:
// its ktw-host.yaml `config:` block is checked in process — a required env key
// unset and a missing tool are reported; set, the key is reported set.
func TestAFolderMembersConfigIsCheckedWithNoProcess(t *testing.T) {
	folder := t.TempDir()
	decl := "name: notes\nconfig:\n  apiKeys:\n    - envName: KTW_TEST_FOLDER_MEMBER_KEY\n      required: true\n" +
		"  tools:\n    - name: nothing\n      commands: [ktw-test-no-such-program]\n"
	if err := os.WriteFile(filepath.Join(folder, "ktw-host.yaml"), []byte(decl), 0o644); err != nil {
		t.Fatal(err)
	}
	ins := mktapi.Installed{Name: "notes", Kind: "content", FolderPath: folder}
	if !isFolderMember(ins) {
		t.Fatal("a content item without scripts/ is a folder member")
	}
	// Any route through the generic host fails: the check runs no process.
	prev := hostBinaryPath
	hostBinaryPath = func() string { return filepath.Join(t.TempDir(), "no-ktw-host.exe") }
	t.Cleanup(func() { hostBinaryPath = prev })
	home := t.TempDir()
	t.Setenv("KTW_TEST_FOLDER_MEMBER_KEY", "")
	rep, err := (&service{}).checkInstalled(context.Background(), home, ins)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Healthy || len(rep.EnvKeys) != 1 || rep.EnvKeys[0].Set || len(rep.Deps) != 1 || rep.Deps[0].OK {
		t.Fatalf("report = %+v; want the key unset and the tool missing, unhealthy", rep)
	}
	t.Setenv("KTW_TEST_FOLDER_MEMBER_KEY", "x")
	rep, err = (&service{}).checkInstalled(context.Background(), home, ins)
	if err != nil || len(rep.EnvKeys) != 1 || !rep.EnvKeys[0].Set {
		t.Fatalf("report = %+v, %v; want the key set", rep, err)
	}
}
