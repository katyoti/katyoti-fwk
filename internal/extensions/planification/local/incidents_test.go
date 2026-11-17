// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE INCIDENTS' LIST - the same stall seen by two
// sweeps is one incident; it closes after two clean sweeps; the list lives in
// the project's own .katyoti, and a sweep writes nothing under modules/.
func TestTheReconcilersIncidentsAreOneListClosedAfterTwoCleanSweeps(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldConfigured(t, nil, reconcilerOn())
	uc := want(t, ctx, cwm, "use-case", "Mail the invoices")
	if _, err := req.File(ctx, api.FileInput{On: uc, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	if err := move(ctx, cwm, uc, api.StateDone, person); err != nil {
		t.Fatal(err)
	}
	journaledDone(t, ctx, uc)
	key := api.RepairEndPlan + ":" + uc
	root := projectRootOf(t, ctx, cwm, uc)
	modules := filepath.Join(root, "modules")

	before := tree(t, modules)
	first := reconcile(t, ctx, api.ReconcileInput{})
	second := reconcile(t, ctx, api.ReconcileInput{})
	if got := tree(t, modules); got != before {
		t.Errorf("two dry-run sweeps wrote under modules/:\nbefore %s\nafter  %s", before, got)
	}
	_ = first
	r := openOn(t, second.Open, key)
	if r == nil || r.Sweeps != 2 {
		t.Fatalf("the stall seen by two sweeps: %+v; want one open incident seen twice (open: %+v)", r, second.Open)
	}
	list := filepath.Join(application.EnvOf(root).HomeDir(), "planification", "incidents.json")
	raw, err := os.ReadFile(list)
	if err != nil {
		t.Fatalf("the incidents' list is not in the project's .katyoti: %v", err)
	}
	if n := strings.Count(string(raw), `"key": "`+key+`"`) + strings.Count(string(raw), `"key":"`+key+`"`); n != 1 {
		t.Errorf("%s holds %s %d times; want once:\n%s", list, key, n, raw)
	}

	// Cleared - its open steps gone, the end of its plan becomes a repair -
	// the incident stays open one clean sweep and closes at the second.
	removeAll(t, ctx, uc)
	third := reconcile(t, ctx, api.ReconcileInput{})
	if r := openOn(t, third.Open, key); r == nil || r.Clean != 1 {
		t.Fatalf("after one clean sweep: %+v; want still open, clean 1 (open: %+v, incidents: %+v)", r, third.Open, third.Incidents)
	}
	fourth := reconcile(t, ctx, api.ReconcileInput{})
	if r := openOn(t, fourth.Open, key); r != nil {
		t.Errorf("after two clean sweeps: %+v; want closed", r)
	}
}

func openOn(t *testing.T, open []api.IncidentRecord, key string) *api.IncidentRecord {
	t.Helper()
	var got *api.IncidentRecord
	for i := range open {
		if open[i].Key == key {
			if got != nil {
				t.Fatalf("%s is open twice: %+v", key, open)
			}
			got = &open[i]
		}
	}
	return got
}

func projectRootOf(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) string {
	t.Helper()
	r, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: id})
	if err != nil || r.ProjectRoot == "" {
		t.Fatalf("the project root of %s: %q %v", id, r.ProjectRoot, err)
	}
	return r.ProjectRoot
}

// tree is every file under a folder with its size and time, to compare.
func tree(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		b.WriteString(p + " " + info.ModTime().String() + "\n")
		return nil
	})
	return b.String()
}
