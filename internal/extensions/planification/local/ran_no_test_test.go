// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A deliverable's go test check that runs no test does not pass: its -run names
// a test that does not exist (not written yet, or renamed) - go test exits 0,
// and the check says it ran none. One that runs a test passes.
func TestACheckThatRanNoTestDoesNotPass(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Count runes", Module: "billing", Kind: api.KindBuild, By: person,
		Deliverables: []api.Deliverable{
			{Title: "a test not written yet", Check: "go test unicode/utf8 -run TestNoSuchTestYet -count=1"},
			{Title: "a test that runs", Check: "go test unicode/utf8 -run TestFullRune -count=1"},
		}})
	out, err := req.Check(ctx, api.CheckInput{ID: r.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range out.Results {
		switch res.Title {
		case "a test not written yet":
			if res.Passed || !strings.Contains(res.Error, "ran no test") {
				t.Errorf("a check running no test: %+v; want not passed, saying it ran none", res)
			}
		case "a test that runs":
			if !res.Passed {
				t.Errorf("a check running a test: %+v; want passed", res)
			}
		}
	}
	for _, d := range out.Request.Deliverables {
		if d.Title == "a test not written yet" && d.Verified {
			t.Error("verified by a check that ran nothing")
		}
	}
}
