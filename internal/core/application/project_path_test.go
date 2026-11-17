// SPDX-License-Identifier: Apache-2.0

package application

import (
	"path/filepath"
	"testing"
)

// TestProjectPathAnchorsOnTheProject: a relative path written by a person or
// an agent lands under the project base whatever the separator style, and an
// absolute path is kept. A sidecar's working directory (its universe folder)
// never decides where a project path points.
func TestProjectPathAnchorsOnTheProject(t *testing.T) {
	base := ProjectBase()
	if base == "" {
		t.Skip("no project base in this environment")
	}
	for _, in := range []string{`modules/erp-acme/.kb`, `modules\erp-acme\.kb`} {
		got, err := ProjectPath(in)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(base, "modules", "erp-acme", ".kb")
		if got != want {
			t.Errorf("ProjectPath(%q) = %q, want %q", in, got, want)
		}
	}
	abs := filepath.Join(t.TempDir(), "kb")
	if got, _ := ProjectPath(abs); got != abs {
		t.Errorf("an absolute path must be kept: got %q, want %q", got, abs)
	}
}
