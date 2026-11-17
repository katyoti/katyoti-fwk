// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE RELEASE PANE ASKS ITS ROWS IN THE GUI'S LANGUAGE:
// the release object pane's source is releases/rows-page, and the GUI calls a
// panel's source with the person's language laid over the object id (the
// SourcePanel of RenderDefView.tsx) - its declared input never overrides it.
// Asked so, its page is French.
func TestTheReleasePaneAsksItsRowsInTheGUIsLanguage(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "contrib", "embed", "cwm", "object-panels", "release.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	pane := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.Contains(pane, "\nsource: \"ktw-agents:releases/rows-page\"\n") {
		t.Fatalf("the release pane's source is not releases/rows-page: %s", pane)
	}
	if strings.Contains(pane, "\ninput:") || strings.Contains(pane, "language:") {
		t.Errorf("the release pane declares an input that would override the GUI's language: %s", pane)
	}

	ctx, _, _ := world(t)
	sm, rel := streams(t, ctx), releases(t, ctx)
	out, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2099-10-31", Stream: out.Stream.ID})
	// The call the GUI makes: the object id, the person's language and level.
	call := func(lang string) string {
		t.Helper()
		var in api.ReleaseRowsPageInput
		if err := json.Unmarshal([]byte(`{"id":"`+r.ID+`","language":"`+lang+`","level":"expert"}`), &in); err != nil {
			t.Fatal(err)
		}
		def, err := rel.RowsPage(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the release pane must display: %v", err)
		}
		b, _ := json.Marshal(def)
		return string(b)
	}
	fr := call("fr")
	for _, want := range []string{"Livré dans cette version (0)", "Rien n'y est encore livré.", `"due":"échéance 2099-10-31`, `"next_label":"La geler"`,
		`"why":"attend une version candidate"`, `{"label":"fait","word":"done"}`} {
		if !strings.Contains(fr, want) {
			t.Errorf("the French release pane lacks %s: %s", want, fr)
		}
	}
	for _, en := range []string{"Ships in this release", "Nothing ships", `"due":"due `, "Freeze it", "needs a candidate", "days left"} {
		if strings.Contains(fr, en) {
			t.Errorf("the French release pane keeps the English %q", en)
		}
	}
}
