// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
)

type stepRecorder struct {
	mu  sync.Mutex
	got []panel.Progress
}

func (r *stepRecorder) Emit(s any) {
	p, ok := s.(panel.Progress)
	if !ok {
		return
	}
	r.mu.Lock()
	r.got = append(r.got, p)
	r.mu.Unlock()
}

func (r *stepRecorder) take() []panel.Progress {
	r.mu.Lock()
	defer r.mu.Unlock()
	got := r.got
	r.got = nil
	return got
}

func labels(ps []panel.Progress) string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Label)
	}
	return strings.Join(out, " | ")
}

// : the marketplace's install-prereqs, install, update
// set-version, install-solution and doctor are declared long, and each
// reports its steps into the panel its run lends it - only the outermost: an
// update's or a solution's installs add no report of their own. Nothing else
// of the surface is long.
func TestTheMarketplaceLongMethodsReportTheirSteps(t *testing.T) {
	long := map[string]bool{"InstallPrereqs": true, "Install": true, "Update": true, "SetVersion": true, "InstallSolution": true, "Doctor": true}
	for name, def := range mktapi.SetupServiceDef.Methods {
		if def.LongRunning != long[name] {
			t.Errorf("%s: declared long = %v, want %v", name, def.LongRunning, long[name])
		}
	}

	s := solutionFixture(t, `      - version: "1"
        includes:
          - {name: notes, version: "1.0"}
          - {name: cards, version: "1.0"}
        universe:
          sidecars: [{name: notes}, {name: cards}]
`)
	// notes 2.0 is in the store too, and offered: an update moves to it
	// without a download.
	folder, err := store.ContentFolderPath("notes", "2.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "ktw-host.yaml"), []byte("name: notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mkts, err := s.reg.List()
	if err != nil || len(mkts) != 1 {
		t.Fatalf("the fixture's marketplace: %v %v", mkts, err)
	}
	raw, err := os.ReadFile(mkts[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	const head = "  - name: notes\n    kind: content\n    versions:\n"
	cat := strings.Replace(string(raw), head, head+"      - version: \"2.0\"\n        assets:\n          - url: notes-2.tar.gz\n", 1)
	if err := os.WriteFile(mkts[0].URL, []byte(cat), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &stepRecorder{}
	ctx := progress.WithEmitter(context.Background(), rec)

	// Install: one step, begun and ended.
	if _, err := s.Install(ctx, mktapi.ItemRef{Name: "notes", Version: "1.0"}); err != nil {
		t.Fatal(err)
	}
	got := rec.take()
	if len(got) != 2 || got[0].Label != "Installing notes@1.0" || got[1].Label != "Installed notes@1.0" || got[1].Current != 1 || got[1].Total != 1 {
		t.Errorf("install reported %+v, want Installing then Installed notes@1.0, 1 of 1", got)
	}

	// SetVersion: one step; the install inside it reports nothing of its own.
	if _, err := s.SetVersion(ctx, mktapi.SetVersionInput{Name: "cards", Version: "1.0"}); err != nil {
		t.Fatal(err)
	}
	if got := rec.take(); len(got) != 2 || got[1].Label != "cards@1.0 is current" {
		t.Errorf("set-version reported %q, want its own step only", labels(got))
	}

	// InstallSolution: each member installed, then the universe made.
	if _, err := s.InstallSolution(ctx, mktapi.InstallSolutionInput{Ref: mktapi.ItemRef{Name: "duo"}, UniverseHome: "solo"}); err != nil {
		t.Fatal(err)
	}
	got = rec.take()
	want := "Installing notes@1.0 | Installed notes@1.0 | Installing cards@1.0 | Installed cards@1.0 | Making the universe solo | Made the universe solo"
	if l := labels(got); l != want {
		t.Errorf("install-solution reported %q, want %q", l, want)
	}
	if last := got[len(got)-1]; last.Current != 3 || last.Total != 3 {
		t.Errorf("install-solution ended at %d of %d, want 3 of 3 (two members, one universe)", last.Current, last.Total)
	}

	// Update: notes moves to 2.0, one step.
	if _, err := s.Update(ctx, mktapi.UpdateInput{Names: []string{"notes"}}); err != nil {
		t.Fatal(err)
	}
	if got := rec.take(); labels(got) != "Updating notes to 2.0 | Updated notes to 2.0" || got[1].Total != 1 {
		t.Errorf("update reported %q (%+v), want one step updating notes to 2.0", labels(got), got)
	}

	// Doctor: one step per installed item checked, whatever its check says.
	if _, err := s.Doctor(ctx, mktapi.DoctorInput{}); err != nil {
		t.Fatal(err)
	}
	got = rec.take()
	if len(got) != 2 || got[1].Current != 2 || got[1].Total != 2 {
		t.Errorf("doctor reported %+v, want one step per installed item (notes, cards)", got)
	}

	// InstallPrereqs: its last step says the checks ran again.
	if _, err := s.InstallPrereqs(ctx, mktapi.PrereqsInstallInput{Universes: []string{"solo"}}); err != nil {
		t.Fatal(err)
	}
	if got := rec.take(); len(got) == 0 || got[len(got)-1].Label != "The sidecars' checks ran again" {
		t.Errorf("install-prereqs reported %q, want its steps ending on the checks run again", labels(got))
	}

	// No reporter: nothing sent, the same work done.
	if _, err := s.Install(context.Background(), mktapi.ItemRef{Name: "cards", Version: "1.0"}); err != nil {
		t.Fatal(err)
	}
	if got := rec.take(); len(got) != 0 {
		t.Errorf("without a reporter the install reported %q", labels(got))
	}
}
