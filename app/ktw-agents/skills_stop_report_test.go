// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

// A LOOP'S STOP REPORT READS « À FAIRE » LIVE: the
// work-loop and module-next skills tell the session to list what waits on the
// person from views person-queue and inbox triage at that moment - never from
// its own notes, where a done or answered item would be listed again.
func TestTheStopReportReadsWhatWaitsLive(t *testing.T) {
	for _, path := range []string{"embed/skills/work-loop.md", "embed/skills/module-next.md"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(raw)
		for _, want := range []string{"views person-queue", "inbox triage", "never from your notes"} {
			if path == "embed/skills/work-loop.md" && want == "never from your notes" {
				want = "Never\nlist an item from the session's own notes"
			}
			if !strings.Contains(s, want) {
				t.Errorf("%s does not say %q", path, want)
			}
		}
	}
}
