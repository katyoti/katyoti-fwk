// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// A RELEASE'S PANE IS BUILT OF SECTIONS (F-098): its
// header and its actions are drawn at once
// and its work - gauges, gates, what awaits a person, its increments, the
// other streams' increments, what runs and what is blocked - is one section
// loading releases/page-section, which answers exactly what the pane built
// at once drew below its actions; an unknown section is refused.
func TestAReleasePaneIsBuiltOfSections(t *testing.T) {
	long.Test(t)
	t.Parallel()
	w := runningIncrementsWorld(t)
	rel := releases(t, w.ctx)
	js := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	def, err := rel.Page(w.ctx, api.GetInput{ID: w.current})
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("the release's pane must display: %v", err)
	}
	var page map[string]any
	_ = json.Unmarshal([]byte(js(def.DSLTree)), &page)
	lazy, _ := page["children"].([]any)
	if len(lazy) < 2 {
		t.Fatalf("the pane has %d parts; want its header, its actions, then its work", len(lazy))
	}
	sec, _ := lazy[len(lazy)-1].(map[string]any)
	load, _ := sec["load"].(map[string]any)
	args, _ := load["args"].(map[string]any)
	if sec["id"] != "release-work" || load["method"] != "ktw-agents:releases/page-section" || args["id"] != w.current || args["section"] != api.ReleasePageWork {
		t.Fatalf("the pane's last part = %s; want the section release-work loading releases/page-section for %s", js(sec), w.current)
	}
	head := lazy[:len(lazy)-1]
	for _, slow := range []string{"Ready to ship?", "Awaiting a person", "Increments of this release", "Running now"} {
		if strings.Contains(js(head), slow) {
			t.Errorf("the pane draws %q before its section loads", slow)
		}
	}

	// The section answers the pane's work below its actions.
	work, err := rel.PageSection(w.ctx, api.ReleasePageSectionInput{ID: w.current, Section: api.ReleasePageWork})
	if err != nil {
		t.Fatal(err)
	}
	if err := work.Validate(); err != nil {
		t.Fatalf("the work section must display: %v", err)
	}
	for _, want := range []string{"Ready to ship?", "Awaiting a person", "Increments of this release", "Running now", "Blocked", `"label":"Increments"`} {
		if !strings.Contains(js(work.DSLTree), want) {
			t.Errorf("PageSection work lacks %q: %s", want, js(work.DSLTree))
		}
	}
	if _, err := rel.PageSection(w.ctx, api.ReleasePageSectionInput{ID: w.current, Section: "everything"}); err == nil {
		t.Error("PageSection answered a section it does not have")
	}
}
