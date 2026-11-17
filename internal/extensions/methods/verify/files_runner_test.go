// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesProbeRunner_Exists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "thing.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := FilesProbeRunner{Root: dir}
	if got := r.RunProbe(context.Background(), "exists:thing.md"); got.Status != "pass" {
		t.Errorf("exists present = %q want pass (%s)", got.Status, got.Evidence)
	}
	if got := r.RunProbe(context.Background(), "exists:nope.md"); got.Status != "fail" {
		t.Errorf("exists missing = %q want fail", got.Status)
	}
	if got := r.RunProbe(context.Background(), "weird:x"); got.Status != "harness-error" {
		t.Errorf("unknown prefix = %q want harness-error", got.Status)
	}
}

func TestFilesProbeRunner_Resolves(t *testing.T) {
	r := FilesProbeRunner{Resolve: func(uri string) bool { return uri == "ktw-x:/skill/y" }}
	if got := r.RunProbe(context.Background(), "resolves:ktw-x:/skill/y"); got.Status != "pass" {
		t.Errorf("resolves hit = %q want pass", got.Status)
	}
	if got := r.RunProbe(context.Background(), "resolves:ktw-x:/skill/missing"); got.Status != "fail" {
		t.Errorf("resolves miss = %q want fail", got.Status)
	}
}
