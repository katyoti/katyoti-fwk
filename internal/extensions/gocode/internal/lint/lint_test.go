// SPDX-License-Identifier: Apache-2.0

package lint

import (
	"testing"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

func TestSidecarMissingAppFolder(t *testing.T) {
	cases := []struct {
		name  string
		mod   gcapi.Module
		wantN int
	}{
		{
			name:  "library module — no check",
			mod:   gcapi.Module{ID: "mod-1", Kind: gcapi.ModuleKindLibrary},
			wantN: 0,
		},
		{
			name:  "sidecar with app_folder — ok",
			mod:   gcapi.Module{ID: "mod-2", Kind: gcapi.ModuleKindSidecar, Facets: []gcapi.Facet{{Kind: gcapi.FacetApp, Path: "app/ktw-foo/"}}},
			wantN: 0,
		},
		{
			name:  "sidecar without app_folder — error",
			mod:   gcapi.Module{ID: "mod-3", Kind: gcapi.ModuleKindSidecar},
			wantN: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, _, _, _ := Run(Inputs{Module: tc.mod}, []string{"SidecarMissingAppFolder"})
			if len(v) != tc.wantN {
				t.Errorf("violations = %d want %d: %+v", len(v), tc.wantN, v)
			}
		})
	}
}

func TestRun_FullSet_FixtureModule(t *testing.T) {
	// Realistic mixed-fixture: a sidecar module mid-stream with
	// some bad UCs. Exercises 4 detectors at once.
	in := Inputs{
		Module: gcapi.Module{
			ID:   "mod-real",
			Name: "sample-sidecar",
			Kind: gcapi.ModuleKindSidecar,
			// AppFolder unset → SidecarMissingAppFolder triggers
		},
		UseCases: []gcapi.UseCase{
			{ID: "uc-good", UCID: "UC-001", Title: "Good UC", Body: "DoD", StatusCache: gcapi.UCStatusCovered},
			{ID: "uc-no-body", UCID: "UC-002", Title: "No DoD", StatusCache: gcapi.UCStatusNoTest},
			{ID: "uc-deferred-bad", UCID: "UC-003", Title: "Deferred no OD", Body: "DoD",
				StatusCache: gcapi.UCStatusFailDeferred},
		},
	}
	v, errCount, _, _ := Run(in, nil)
	// Expect at least 3 errors (sidecar app_folder + no-body + deferred-without-OD).
	if errCount < 3 {
		t.Errorf("errCount = %d want ≥3 (sidecar+nobody+deferredbad); violations: %+v", errCount, v)
	}
}

func TestRun_NamedSubset(t *testing.T) {
	in := Inputs{
		Module: gcapi.Module{ID: "mod-1", Kind: gcapi.ModuleKindSidecar},
		UseCases: []gcapi.UseCase{
			{ID: "uc-1"}, // missing body
		},
	}
	// Running only OrphanUseCases means the no-body uc + sidecar
	// app_folder errors are skipped.
	v, errCount, _, _ := Run(in, []string{"OrphanUseCases"})
	if errCount != 0 {
		t.Errorf("named subset should isolate detector — errCount=%d", errCount)
	}
	for _, vi := range v {
		if vi.Detector != "OrphanUseCases" {
			t.Errorf("named subset leaked: got Detector=%q", vi.Detector)
		}
	}
}
