// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"slices"
	"testing"
)

// THE CHANGE LOGS A LETTER COVERS REACH THE SNAPSHOT: what
// the marketplace build names - --whats-new, --covers, the solution's
// letter_covers - is handed on as the snapshot's options, so the figures are
// checked against every one.
func TestTheLetterCoversReachTheSnapshot(t *testing.T) {
	covers := []string{"C:/def/changelog/2026.10.md", "C:/def/changelog/1.006.md"}
	o := snapshotOptions(SolutionInput{WhatsNew: covers[0], LetterCovers: covers, LetterRequests: 4})
	if o.WhatsNew != covers[0] || !slices.Equal(o.LetterCovers, covers) {
		t.Errorf("the snapshot's options: whats-new %q, covers %v; want %q and %v", o.WhatsNew, o.LetterCovers, covers[0], covers)
	}
	if o.LetterRequests != 4 {
		t.Errorf("the requests their changes name: %d; want 4", o.LetterRequests)
	}
}
