// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildableLeavesOutTestOnlyPackages: go build refuses a folder of tests
// only, so the correctness check builds the others and the "..." patterns
// (go vet and go test still cover the test-only folder).
func TestBuildableLeavesOutTestOnlyPackages(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("code/a.go")
	write("code/a_test.go")
	write("testsonly/guard_test.go")
	code := filepath.ToSlash(filepath.Join(dir, "code"))
	tests := filepath.ToSlash(filepath.Join(dir, "testsonly"))
	empty := filepath.ToSlash(filepath.Join(dir, "missing"))
	got := buildable([]string{code, tests, empty, "./internal/x/..."})
	if strings.Join(got, ",") != code+",./internal/x/..." {
		t.Errorf("buildable = %v, want [%s ./internal/x/...]", got, code)
	}
	if got := buildable([]string{tests}); len(got) != 0 {
		t.Errorf("a test-only folder alone = %v, want nothing to build", got)
	}
}

// TestVetLeavesOutTagOnlyPackages: go vet and go test refuse a named folder
// whose Go files a build tag all leaves out ("build constraints exclude all
// Go files"), so the correctness and quality checks leave it out - go list
// says which - and keep the others and the "..." patterns.
func TestVetLeavesOutTagOnlyPackages(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/tagonly\n\ngo 1.21\n")
	write("code/a.go", "package code\n")
	write("tagged/e2e_test.go", "//go:build sometag\n\npackage tagged\n")
	got := leaveOutConstrained(context.Background(), dir, []string{"./code", "./tagged", "./sub/..."})
	if strings.Join(got, ",") != "./code,./sub/..." {
		t.Errorf("leaveOutConstrained = %v, want [./code ./sub/...]", got)
	}
	if got := leaveOutConstrained(context.Background(), dir, []string{"./tagged"}); len(got) != 0 {
		t.Errorf("a tag-only folder alone = %v, want nothing to vet", got)
	}
}
