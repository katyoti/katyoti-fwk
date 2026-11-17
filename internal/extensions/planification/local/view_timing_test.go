// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// viewBudget is how long a planning view may take to open, warm: two seconds,
// the person's decision of 2026-11-02 (« every timing under 2 seconds is
// okay »).
const viewBudget = 2 * time.Second

// EVERY PLANNING VIEW OPENS IN UNDER TWO SECONDS, WARM (
// after F-056's two), built of sections - a
// tab a person opens less often loads when it opens -, on a copy of this
// repository's modules - the development workspace's
// objects and planning state, never the workspace itself: the deck, « À
// faire », the backlog, the next release's page and an increment's page, each
// timed on its first open once the copy is indexed. On demand only (it copies
// the modules - some 80 MB - and indexes them):
//
//	KTW_VIEW_TIMING=1 go test ./internal/extensions/planification/local/ -run TestPlanningViewsOpenInUnderTwoSeconds -count=1 -v -timeout 20m
func TestPlanningViewsOpenInUnderTwoSeconds(t *testing.T) {
	if os.Getenv("KTW_VIEW_TIMING") == "" {
		t.Skip("on demand: KTW_VIEW_TIMING=1 (copies modules/ and indexes it)")
	}
	ctx, cwm, _ := world(t)
	// The world's project: where its own module lives.
	at, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: "billing"})
	if err != nil {
		t.Fatalf("find the world's project: %v", err)
	}
	modules := at.Path
	for filepath.Base(modules) != "modules" && filepath.Dir(modules) != modules {
		modules = filepath.Dir(modules)
	}
	if err := copyModules(filepath.Join("..", "..", "..", "..", "modules"), modules); err != nil {
		t.Fatalf("copy the modules: %v", err)
	}
	start := time.Now()
	if _, err := cwm.ObjectScan(ctx, cwmapi.ObjectScanInput{}); err != nil {
		t.Fatalf("index the copy: %v", err)
	}
	t.Logf("indexed in %v", time.Since(start).Round(time.Millisecond))

	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	rel, inc := releases(t, ctx), increments(t, ctx)
	next, increment := "", ""
	if rs, err := rel.List(ctx, api.ListInput{}); err == nil {
		for _, sm := range []string{"roadmap.SM-001", ""} {
			var in []api.Release
			for _, r := range rs.Releases {
				if sm == "" || r.Stream == sm {
					in = append(in, r)
				}
			}
			if r, ok := api.NextPlanned(in); ok {
				next = r.ID
				break
			}
		}
	}
	if is, err := inc.List(ctx, api.ListIncrementsInput{State: api.IncrementOpen}); err == nil {
		for _, i := range is.Increments {
			if increment == "" || (next != "" && i.ShipsIn == next) {
				increment = i.ID
			}
		}
	}

	type view struct {
		name string
		open func() (render.Def, error)
	}
	timed := []view{
		{"the planning deck", func() (render.Def, error) { return views.Deck(ctx, api.DeckInput{}) }},
		{"« À faire »", func() (render.Def, error) { return views.PersonQueuePage(ctx, api.PersonQueueInput{}) }},
		{"the Delivery view", func() (render.Def, error) { return streams(t, ctx).DeliveryPage(ctx, api.DeliveryPageInput{}) }},
	}
	if next != "" {
		// Its pane and then its work section, as the GUI loads them
		// : the page is open once both have answered.
		timed = append(timed, view{"the next release's page (" + next + ")", func() (render.Def, error) {
			def, err := rel.Page(ctx, api.GetInput{ID: next})
			if err != nil {
				return def, err
			}
			if _, err := rel.PageSection(ctx, api.ReleasePageSectionInput{ID: next, Section: api.ReleasePageWork}); err != nil {
				return def, err
			}
			return def, nil
		}})
	}
	if increment != "" {
		timed = append(timed, view{"an increment's pane (" + increment + ")", func() (render.Def, error) { return inc.PanePage(ctx, api.IncrementPaneInput{ID: increment}) }})
	}
	// Warm: the budget is a view opened again in a running universe; the
	// first opening, cold, is logged only.
	for _, v := range timed {
		start := time.Now()
		if _, err := v.open(); err == nil {
			t.Logf("%s, cold: %v", v.name, time.Since(start).Round(time.Millisecond))
		}
	}
	for _, v := range timed {
		start := time.Now()
		def, err := v.open()
		took := time.Since(start)
		if err != nil {
			t.Errorf("%s: %v", v.name, err)
			continue
		}
		t.Logf("%s: %v", v.name, took.Round(time.Millisecond))
		if took > viewBudget {
			t.Errorf("%s took %v; want under %v", v.name, took.Round(time.Millisecond), viewBudget)
		}
		if def.Kind == "" {
			t.Errorf("%s answered no page", v.name)
		}
	}
}

// copyModules copies the repository's modules into the world's: every file
// but a secrets file (.env*), never overwriting the world's own.
func copyModules(from, to string) error {
	return filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if strings.HasPrefix(info.Name(), ".env") {
			return nil
		}
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}
