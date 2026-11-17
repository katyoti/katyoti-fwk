// SPDX-License-Identifier: Apache-2.0

package configset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NO members.yaml MEANS NO MEMBERSHIP OPINION — not an error.
//
// Every config-set published before this file existed has none. If a missing
// file were an error, adding the format would break every bundle already in a
// catalog, which is the one thing a format addition may not do.
func TestLoadSolution_MissingFileIsNotAnError(t *testing.T) {
	sol, err := LoadMembers(t.TempDir())
	if err != nil {
		t.Fatalf("missing members.yaml: %v — a bundle without one must apply as it always did", err)
	}
	if len(sol.Sidecars) != 0 {
		t.Errorf("sidecars = %v, want none", sol.Sidecars)
	}
}

// AN EMPTY LIST IS THE SAME AS NO FILE.
func TestLoadSolution_EmptyListWiresNothing(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "sidecars: []\n")
	sol, err := LoadMembers(dir)
	if err != nil {
		t.Fatalf("LoadMembers: %v", err)
	}
	if len(sol.Sidecars) != 0 {
		t.Errorf("sidecars = %v, want none", sol.Sidecars)
	}
}

// NAMES ONLY, AND DUPLICATES COLLAPSE.
//
// Membership is a SET: several config-sets applied to one universe contribute a
// union, so a name repeated inside one list cannot mean "twice".
func TestLoadSolution_ReadsNamesAsASet(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "sidecars:\n  - ktw-example\n  - ktw-docs\n  - ktw-example\n  - \"\"\n")
	sol, err := LoadMembers(dir)
	if err != nil {
		t.Fatalf("LoadMembers: %v", err)
	}
	want := []string{"ktw-example", "ktw-docs"}
	if len(sol.Sidecars) != len(want) {
		t.Fatalf("sidecars = %v, want %v", sol.Sidecars, want)
	}
	for i := range want {
		if sol.Sidecars[i] != want[i] {
			t.Fatalf("sidecars = %v, want %v", sol.Sidecars, want)
		}
	}
}

// A VERSIONED NAME IS REFUSED, AND SAYS WHY.
//
// `- ktw-example@v1` looks like it pins. It does not: apply resolves the latest
// of whatever is missing, so the author would get a different build than the
// one they wrote down and nothing would tell them. Pinning is what kind:release
// is for, and the error says so rather than trimming the version silently.
func TestLoadSolution_RefusesAVersionedName(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "sidecars:\n  - ktw-example@v1\n")
	if _, err := LoadMembers(dir); err == nil {
		t.Fatal("a sidecar name carrying @version was accepted — it would resolve to something else")
	}
}

func write(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, MembersFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A bundle whose member list has its former name is refused at publish,
// saying to rename it (it is still read at install).
func TestValidateRefusesTheFormerMembersName(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "sidecars: [ktw-x]\n")
	_ = os.Rename(filepath.Join(dir, MembersFile), filepath.Join(dir, LegacyMembersFile))
	if _, err := Validate(dir); err == nil || !strings.Contains(err.Error(), MembersFile) {
		t.Fatalf("Validate = %v, want a refusal naming %s", err, MembersFile)
	}
	if m, err := LoadMembers(dir); err != nil || len(m.Sidecars) != 1 {
		t.Fatalf("the former name should still be read: %+v %v", m, err)
	}
}
