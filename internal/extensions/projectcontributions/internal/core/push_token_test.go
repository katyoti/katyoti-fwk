// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"
)

// An object whose project does not resolve is pushed under the attached
// project's token only when exactly one project is attached and served: with
// several, it is not pushed under the first one's.
func TestAnUnresolvedObjectFallsBackOnlyToALoneAttachedProject(t *testing.T) {
	repo := t.TempDir()
	alpha := Project(t, repo, "alpha", "alpha", true)
	beta := Project(t, repo, "beta", "beta", true)
	bare := Project(t, repo, "bare", "bare", false)
	if got := fallbackToken([]string{alpha}, servedTokens([]string{alpha})); got != "alpha" {
		t.Fatalf("one project: token = %q, want alpha", got)
	}
	if got := fallbackToken([]string{alpha, beta}, servedTokens([]string{alpha, beta})); got != "" {
		t.Fatalf("several projects: token = %q, want none", got)
	}
	if got := fallbackToken([]string{bare}, servedTokens([]string{bare})); got != "" {
		t.Fatalf("a project with no contributions folder: token = %q, want none", got)
	}
	if got := fallbackToken(nil, nil); got != "" {
		t.Fatalf("no project: token = %q, want none", got)
	}
}
