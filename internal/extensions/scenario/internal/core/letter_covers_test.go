// SPDX-License-Identifier: Apache-2.0

package core

import (
	"slices"
	"testing"
)

// THE CHANGE LOGS A LETTER COVERS REACH THE SOLUTION: the
// snapshot hands its « Quoi de neuf » and every change log it covers to the
// solution, whose build checks the letter's figures against all of them.
func TestTheLetterCoversReachTheSolution(t *testing.T) {
	covers := []string{"C:/def/changelog/2026.10.md", "C:/def/changelog/1.006.md"}
	spec := solutionSpec(SnapshotOptions{WhatsNew: covers[0], LetterCovers: covers, LetterRequests: 4})
	if spec.WhatsNew != covers[0] || !slices.Equal(spec.LetterCovers, covers) {
		t.Errorf("the solution spec: whats-new %q, covers %v; want %q and %v", spec.WhatsNew, spec.LetterCovers, covers[0], covers)
	}
	if spec.LetterRequests != 4 {
		t.Errorf("the requests their changes name: %d; want 4", spec.LetterRequests)
	}
}
