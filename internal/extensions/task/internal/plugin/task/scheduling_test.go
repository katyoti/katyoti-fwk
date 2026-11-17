// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// Migrated 2026-05-07 from in-package requestFieldsFromMode calls to
// presets.SchedulingFieldsFromMode per the OD-001 spawn-policy refactor.

func TestRequestFieldsFromMode_Worker(t *testing.T) {
	maxDurS, fireForget := presets.SchedulingFieldsFromMode(presets.ModeWorker)
	if maxDurS != 0 {
		t.Errorf("worker MaxDurationS = %d, want 0 (unlimited)", maxDurS)
	}
	if fireForget {
		t.Error("worker should not be fire-and-forget")
	}
}

func TestRequestFieldsFromMode_Background(t *testing.T) {
	_, fireForget := presets.SchedulingFieldsFromMode(presets.ModeBackground)
	// background is async but still gets notification — not fire-and-forget.
	if fireForget {
		t.Error("background should not be fire-and-forget")
	}
}

func TestRequestFieldsFromMode_UnknownReturnsZero(t *testing.T) {
	maxDurS, fireForget := presets.SchedulingFieldsFromMode("no-such-mode")
	if maxDurS != 0 || fireForget {
		t.Errorf("unknown mode should return zero values, got MaxDurationS=%d FireAndForget=%v", maxDurS, fireForget)
	}
}
