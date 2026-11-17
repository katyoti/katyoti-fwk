// SPDX-License-Identifier: Apache-2.0

package lint

import (
	"strings"
	"testing"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

func TestMissingDoDBody(t *testing.T) {
	in := Inputs{
		Module: gcapi.Module{ID: "mod-1"},
		UseCases: []gcapi.UseCase{
			{ID: "uc-1", UCID: "UC-001", Title: "Has body", Body: "DoD content"},
			{ID: "uc-2", UCID: "UC-002", Title: "No body"},
			{ID: "uc-3", UCID: "UC-003"}, // no title either
		},
	}
	v, errCount, _, _ := Run(in, []string{"MissingDoDBody"})
	if errCount != 2 {
		t.Errorf("errCount = %d want 2 (uc-2 + uc-3)", errCount)
	}
	if len(v) != 2 {
		t.Fatalf("violations = %d want 2", len(v))
	}
	// Detector name + severity stamped.
	for _, vi := range v {
		if vi.Detector != "MissingDoDBody" {
			t.Errorf("Detector = %q", vi.Detector)
		}
		if vi.Severity != SeverityError {
			t.Errorf("Severity = %q want error", vi.Severity)
		}
	}
}

func TestDeferredWithoutOD(t *testing.T) {
	in := Inputs{
		Module: gcapi.Module{ID: "mod-1"},
		UseCases: []gcapi.UseCase{
			{ID: "uc-1", UCID: "UC-001", StatusCache: gcapi.UCStatusCovered},
			{ID: "uc-2", UCID: "UC-002", StatusCache: gcapi.UCStatusFailDeferred, FailDeferredBy: "OD-001"},
			{ID: "uc-3", UCID: "UC-003", StatusCache: gcapi.UCStatusFailDeferred},
		},
	}
	v, errCount, _, _ := Run(in, []string{"DeferredWithoutOD"})
	if errCount != 1 {
		t.Errorf("errCount = %d want 1 (uc-3 missing OD)", errCount)
	}
	if len(v) != 1 || v[0].ItemID != "uc-3" {
		t.Errorf("expected single violation on uc-3, got %+v", v)
	}
}

func TestOrphanUseCases(t *testing.T) {
	in := Inputs{
		Module: gcapi.Module{ID: "mod-1"},
		UseCases: []gcapi.UseCase{
			{ID: "uc-1", StatusCache: gcapi.UCStatusCovered},
			{ID: "uc-2", StatusCache: gcapi.UCStatusNoTest},
			{ID: "uc-3", StatusCache: gcapi.UCStatusNoTest},
		},
	}
	v, errCount, warnCount, _ := Run(in, []string{"OrphanUseCases"})
	if errCount != 0 {
		t.Errorf("errCount = %d want 0 (orphans are warn-only)", errCount)
	}
	if warnCount != 2 {
		t.Errorf("warnCount = %d want 2", warnCount)
	}
	if len(v) != 2 {
		t.Fatalf("violations = %d want 2", len(v))
	}
}

func TestStatusCacheDrift(t *testing.T) {
	in := Inputs{
		Module: gcapi.Module{ID: "mod-1"},
		UseCases: []gcapi.UseCase{
			// has status but no last-verified + no test-runs → drift
			{ID: "uc-1", StatusCache: gcapi.UCStatusCovered},
			// status cleared / no-test → not a drift candidate
			{ID: "uc-2"},
			{ID: "uc-3", StatusCache: gcapi.UCStatusNoTest},
		},
		TestRuns: nil,
	}
	v, _, warnCount, _ := Run(in, []string{"StatusCacheDrift"})
	if warnCount != 1 {
		t.Errorf("warnCount = %d want 1 (uc-1 drift)", warnCount)
	}
	if len(v) != 1 || v[0].ItemID != "uc-1" {
		t.Errorf("expected single drift on uc-1, got %+v", v)
	}
}

func TestStatusCacheDrift_NoDriftWhenTestRunsPresent(t *testing.T) {
	in := Inputs{
		Module: gcapi.Module{ID: "mod-1"},
		UseCases: []gcapi.UseCase{
			{ID: "uc-1", StatusCache: gcapi.UCStatusCovered},
		},
		TestRuns: []gcapi.TestRun{{ID: "tr-1"}},
	}
	v, _, warnCount, _ := Run(in, []string{"StatusCacheDrift"})
	if warnCount != 0 {
		t.Errorf("warnCount = %d want 0 (test-run exists)", warnCount)
	}
	if len(v) != 0 {
		t.Errorf("expected no violations, got %+v", v)
	}
}

func TestCharterAndManifestDrift(t *testing.T) {
	in := Inputs{
		Module:        gcapi.Module{ID: "mod-1"},
		CharterDrift:  true,
		ManifestDrift: true,
	}
	v, _, warnCount, _ := Run(in, nil) // run all detectors
	if warnCount < 2 {
		t.Errorf("warnCount = %d want ≥2 (charter + manifest drift)", warnCount)
	}
	hasCharter, hasManifest := false, false
	for _, vi := range v {
		if vi.Detector == "CharterProjectionDrift" {
			hasCharter = true
			if !strings.Contains(vi.Reason, "charter.md") {
				t.Errorf("CharterProjectionDrift reason %q lacks 'charter.md'", vi.Reason)
			}
		}
		if vi.Detector == "ManifestDrift" {
			hasManifest = true
		}
	}
	if !hasCharter || !hasManifest {
		t.Errorf("missing drift violations: charter=%v manifest=%v", hasCharter, hasManifest)
	}
}
