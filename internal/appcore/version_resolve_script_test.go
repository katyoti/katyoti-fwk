// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Go binaries resolve "dev" at RUNTIME from debug.ReadBuildInfo; the publish
// scripts resolve it at BUILD time in shell. Two implementations of one rule.
//
// They diverged once already, and the symptom was subtle rather than loud: a
// single `make release` published the control plane as "dev" while the catalog
// published its sidecars as "a1b2c3d4-dirty". Nothing failed — the release was
// simply unable to answer which infra went with which sidecars, which is the
// question a compatibility contract is built on.
//
// scripts/resolve-version.sh is now the single answer for the scripts. This
// test pins it to the Go one, so the two cannot drift apart again silently.
func TestResolveVersionScriptMatchesGoRule(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no POSIX shell available")
	}
	root := repoRoot(t)
	rev, err := runIn(t, root, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Skip("not a git checkout — the VCS-derived path cannot be exercised")
	}

	// Build the expectation with ResolveVersion's OWN rule (see
	// version_resolve.go): first 8 characters of the revision, plus "-dirty"
	// when the tree is modified.
	//
	// Deriving it here rather than reading this test binary's build info is
	// deliberate: `go test` binaries are commonly built without VCS stamping,
	// so that version of the test skipped every time — a test that never runs
	// is worse than no test, because it looks like coverage.
	want := strings.TrimSpace(rev)
	if len(want) > 8 {
		want = want[:8]
	}
	porcelain, err := runIn(t, root, "git", "status", "--porcelain")
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if strings.TrimSpace(porcelain) != "" {
		want += "-dirty"
	}

	got, err := runIn(t, root, "sh", "scripts/resolve-version.sh")
	if err != nil {
		t.Fatalf("resolve-version.sh: %v", err)
	}
	if strings.TrimSpace(got) != want {
		t.Errorf("scripts/resolve-version.sh = %q, want %q (appcore.ResolveVersion's rule)\n"+
			"one build must produce ONE version string, or a release cannot say which infra matches which sidecars",
			strings.TrimSpace(got), want)
	}
}

// An explicit version must be passed through untouched by both sides: a release
// stamps a semver from a git tag, and rewriting it would detach the published
// version from the tag it names.
func TestResolveVersionScriptPassesExplicitThrough(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no POSIX shell available")
	}
	root := repoRoot(t)
	cmd := exec.Command("sh", "scripts/resolve-version.sh")
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "VERSION=v9.9.9")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("resolve-version.sh: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "v9.9.9" {
		t.Errorf("script rewrote an explicit version: got %q, want v9.9.9", got)
	}
	if got := ResolveVersion("v9.9.9"); got != "v9.9.9" {
		t.Errorf("ResolveVersion rewrote an explicit version: got %q, want v9.9.9", got)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test's source file")
	}
	return filepath.Join(filepath.Dir(self), "..", "..")
}

func runIn(t *testing.T, dir, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}
