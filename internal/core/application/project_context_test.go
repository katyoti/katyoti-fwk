// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"path/filepath"
	"testing"
)

// TestProjectContext: WithProject stamps a project; ProjectEnv resolves that
// project's HomeDir (per-project), while an unstamped ctx falls back to the
// singleton. Empty project is a no-op.
func TestProjectContext(t *testing.T) {
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	ResetGlobals()
	t.Cleanup(ResetGlobals)

	base := context.Background()
	if ProjectFromContext(base) != "" {
		t.Fatal("bare ctx should carry no project")
	}
	// empty is a no-op
	if WithProject(base, "") != base {
		t.Fatal("WithProject(\"\") should be a no-op")
	}

	projA := t.TempDir()
	ctx := WithProject(base, projA)
	if got := ProjectFromContext(ctx); got != projA {
		t.Fatalf("ProjectFromContext = %q, want %q", got, projA)
	}
	// ProjectEnv resolves the ctx project's HomeDir (project-scoped)
	wantHome := filepath.Join(projA, ".katyoti")
	if got := ProjectEnv(ctx).HomeDir(); got != wantHome {
		t.Fatalf("ProjectEnv(ctx).HomeDir = %q, want %q", got, wantHome)
	}
	// no project on ctx → ProjectEnv falls back to the singleton Env()
	if ProjectEnv(base).HomeDir() != Env().HomeDir() {
		t.Fatal("ProjectEnv(bare) should fall back to the singleton Env()")
	}
}
