// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"strings"
	"testing"
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestAnAuditRanWhenItsRunSays: when an audit ran is the run's own record —
// the instant its review is named after, else the review's date — never a
// time the object stores; with neither, nothing.
func TestAnAuditRanWhenItsRunSays(t *testing.T) {
	review := func(id string, fields map[string]any) cwmapi.Object {
		o := cwmapi.Object{ID: id, Type: gcapi.AuditReviewType, Parent: "m", Fields: fields}
		return o
	}
	cases := []struct {
		name string
		o    cwmapi.Object
		want time.Time
	}{
		{"its instant", review("m.audit-2026-10-08T09-15-30", map[string]any{"date": "2026-10-07"}), time.Date(2026, 10, 8, 9, 15, 30, 0, time.UTC)},
		{"its date", review("m.audit-1", map[string]any{"date": "2026-10-07"}), time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)},
		{"nothing", review("m.audit-1", nil), time.Time{}},
	}
	for _, c := range cases {
		if got := decodeAuditReview(c.o).RanAt; !got.Equal(c.want) {
			t.Errorf("%s: RanAt = %s, want %s", c.name, got, c.want)
		}
	}
}

// TestTheCharterSaysNoTimeItDoesNotKnow: an audit with no run time prints no
// "ran", never the zero date.
func TestTheCharterSaysNoTimeItDoesNotKnow(t *testing.T) {
	in := charterFixture()
	in.LatestAudit = &gcapi.AuditBundle{Name: "audit-x"}
	out := BuildCharter(in)
	if !strings.Contains(out, "**Latest**: `audit-x`") || strings.Contains(out, "0001-01-01") || strings.Contains(out, "ran ") {
		t.Errorf("audit history without a run time:\n%s", out)
	}
	if out := BuildCharter(charterFixture()); !strings.Contains(out, "ran 2026-05-18T09:00:00Z") {
		t.Errorf("audit history with a run time lost it:\n%s", out)
	}
}
