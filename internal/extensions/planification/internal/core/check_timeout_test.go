// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/projectcmd"
)

// TestADeliverableCheckRunsToItsTimeout: a deliverable's command check longer
// than the default bound runs to the bound it declares and passes; without
// one the same check is stopped at the default; a bound above the ceiling is
// refused and the check never runs.
func TestADeliverableCheckRunsToItsTimeout(t *testing.T) {
	if _, err := projectcmd.FindBash(); err != nil {
		t.Skipf("no bash on this machine: %v", err)
	}
	keep := checkTimeout
	checkTimeout = time.Second
	t.Cleanup(func() { checkTimeout = keep })
	ctx := context.Background()
	slow := "sleep 3 && echo slow check done"

	if out, err := runCheck(ctx, slow, "20s"); err != nil || !strings.Contains(out, "slow check done") {
		t.Fatalf("declared 20s: out %q err %v, want it to pass", out, err)
	}
	if _, err := runCheck(ctx, slow, ""); err == nil || !strings.Contains(err.Error(), "timed out after 1s") {
		t.Fatalf("no timeout: err %v, want it stopped at the default 1s", err)
	}
	start := time.Now()
	if _, err := runCheck(ctx, slow, "2h"); err == nil || !strings.Contains(err.Error(), "above the ceiling") {
		t.Fatalf("declared 2h: err %v, want it refused above the ceiling", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("a check whose bound is refused was run")
	}
}
