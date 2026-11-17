// SPDX-License-Identifier: Apache-2.0

package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// project makes a project folder named name under parent with a
// contributions folder (the default modules/sys-contributions): no project
// file, so its token is its folder's name.
func project(t *testing.T, parent, name string, contributes bool) string {
	t.Helper()
	root := filepath.Join(parent, name)
	dir := root
	if contributes {
		dir = filepath.Join(root, "modules", "sys-contributions")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// Every attached project with a contributions folder is a member under its
// own token; one without a folder, or missing on disk, gives none.
func TestEveryAttachedProjectIsAMemberUnderItsOwnToken(t *testing.T) {
	tmp := t.TempDir()
	a := project(t, tmp, "alpha", true)
	b := project(t, tmp, "beta", true)
	c := project(t, tmp, "gamma", false)
	got, refused := ProjectMembers([]string{a, filepath.Join(tmp, "gone"), c, b}, nil)
	if len(refused) != 0 {
		t.Fatalf("refused = %+v", refused)
	}
	if len(got) != 2 || got[0].Token != "alpha" || got[1].Token != "beta" {
		t.Fatalf("members = %+v", got)
	}
	if got[1].Project != b || got[1].Folder != filepath.Join(b, "modules", "sys-contributions") {
		t.Fatalf("beta = %+v", got[1])
	}
}

// Two projects under one token: the first attached is served, the second is
// refused with a finding naming the token and both projects.
func TestASecondProjectUnderAHeldTokenIsRefusedNamingBoth(t *testing.T) {
	tmp := t.TempDir()
	first := project(t, filepath.Join(tmp, "one"), "acme", true)
	second := project(t, filepath.Join(tmp, "two"), "acme", true)
	got, refused := ProjectMembers([]string{first, second}, nil)
	if len(got) != 1 || got[0].Project != first {
		t.Fatalf("members = %+v", got)
	}
	if len(refused) != 1 || refused[0].Project != second || refused[0].Token != "acme" {
		t.Fatalf("refused = %+v", refused)
	}
	for _, want := range []string{`"acme"`, first, second} {
		if !strings.Contains(refused[0].Why, want) {
			t.Fatalf("finding %q does not name %s", refused[0].Why, want)
		}
	}
}

// A reserved token or a declared member's name is refused, never shadowing.
func TestAReservedOrDeclaredTokenIsRefused(t *testing.T) {
	tmp := t.TempDir()
	declared := project(t, tmp, "ktw-compose", true)
	reserved := project(t, tmp, "ktw-hub", true)
	got, refused := ProjectMembers([]string{declared, reserved}, []string{"ktw-compose"})
	if len(got) != 0 || len(refused) != 2 {
		t.Fatalf("members = %+v, refused = %+v", got, refused)
	}
	if !strings.Contains(refused[0].Why, "member ktw-compose") || !strings.Contains(refused[1].Why, "reserved") {
		t.Fatalf("refused = %+v", refused)
	}
}
