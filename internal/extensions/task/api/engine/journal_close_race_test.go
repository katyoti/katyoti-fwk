// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"sync"
	"testing"
)

// A journal closed while events are still being logged to it (a parent's
// session closing while its stopped children still report) neither races nor
// writes after the close. Run under -race.
func TestAJournalClosedWhileLoggingIsSafe(t *testing.T) {
	r, err := NewSessionRecorder(t.TempDir(), "t", "s")
	if err != nil {
		t.Fatal(err)
	}
	j := r.LLMJournal()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 200; k++ {
				j.Log("probe", map[string]any{"k": k})
			}
		}()
	}
	_ = r.Close()
	wg.Wait()
}
