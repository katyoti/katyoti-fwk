// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// A ROW WAITING ON SEVERAL REQUESTS SAYS WHAT IT SHOWS:
// its step-and-who, read by the GUI test, is the row's own words - « waits
// on » then each request it waits on, as its chips follow the word - with no
// separator the row does not draw.
func TestARowWaitingOnSeveralRequestsSaysWhatItShows(t *testing.T) {
	row := requestRow("en", paneRequest{ID: "m.RQ-3", Title: "Ship it", Bucket: "waits", On: []string{"m.RQ-1", "m.RQ-2"}}, openStep{}, rowOpts{})
	b, _ := json.Marshal(row)
	raw := string(b)
	if !strings.Contains(raw, `"data-step-who":"waits on m.RQ-1 m.RQ-2"`) {
		t.Errorf("its step-and-who is not the row's words « waits on m.RQ-1 m.RQ-2 »: %s", raw)
	}
	for _, chip := range []string{"m.RQ-1", "m.RQ-2"} {
		if !strings.Contains(raw, `"data-open":"`+chip+`"`) {
			t.Errorf("no chip opening %s: %s", chip, raw)
		}
	}
	if strings.Contains(raw, `"content":", "`) || strings.Contains(raw, "m.RQ-1, ") {
		t.Errorf("the row draws a separator its chips do not show: %s", raw)
	}
}
