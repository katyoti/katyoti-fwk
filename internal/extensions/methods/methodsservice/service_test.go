// SPDX-License-Identifier: Apache-2.0

package methodsservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyAcceptance(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "present.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &service{repoRoot: dir}
	ctx := context.Background()

	// No criteria → fail (not-acceptance, never a silent pass).
	if r, _ := s.VerifyAcceptance(ctx, AcceptanceInput{}); r.Status != "fail" {
		t.Errorf("empty criteria should fail, got %q", r.Status)
	}

	// All probes pass → pass.
	pass := AcceptanceInput{Criteria: []AcceptanceCriterion{
		{ID: "AC-1", TestRef: "exists:present.txt"},
	}}
	if r, _ := s.VerifyAcceptance(ctx, pass); r.Status != "pass" {
		t.Errorf("present file should pass, got %q (%s)", r.Status, r.Evidence)
	}

	// A missing-file probe → fail, evidence names the criterion.
	mixed := AcceptanceInput{Criteria: []AcceptanceCriterion{
		{ID: "AC-1", TestRef: "exists:present.txt"},
		{ID: "AC-2", TestRef: "exists:absent.txt"},
	}}
	r, _ := s.VerifyAcceptance(ctx, mixed)
	if r.Status != "fail" {
		t.Errorf("a missing file should fail the set, got %q", r.Status)
	}
	if !contains(r.Evidence, "AC-2") {
		t.Errorf("evidence should name the failing criterion AC-2, got %q", r.Evidence)
	}

	// A criterion with no test_ref → fail (can't be auto-proven).
	noRef := AcceptanceInput{Criteria: []AcceptanceCriterion{{ID: "AC-9"}}}
	if r, _ := s.VerifyAcceptance(ctx, noRef); r.Status != "fail" {
		t.Errorf("a criterion without test_ref should fail, got %q", r.Status)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
