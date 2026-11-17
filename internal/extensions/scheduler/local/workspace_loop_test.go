// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"fmt"
	"sync"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// SEVERAL SESSIONS ON THE WHOLE WORKSPACE — each pulls the next takeable to-do
// of any module (no module named), never one another holds; one at a time
// ; nothing once the work is gone.

func TestWorkersOnTheWholeWorkspaceNeverTakeTheSameToDo(t *testing.T) {
	ctx, _, svc, _ := world(t)
	want := map[string]bool{}
	for i, m := range []string{"billing", "shipping", "billing"} {
		want[put(t, ctx, svc, api.Todo{Module: m, Title: fmt.Sprintf("step %d", i)}).ID] = true
	}
	workers := make([]cwmapi.Actor, 5)
	for i := range workers {
		workers[i] = cwmapi.Actor{ID: fmt.Sprintf("claude-%d", i), Kind: "agent"}
	}

	// All at once, no module: three get one each, never the same; two get nothing.
	got := make([]*api.Todo, len(workers))
	var wg sync.WaitGroup
	for i, w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := svc.Pull(ctx, api.PullInput{By: w})
			if err != nil {
				t.Error(err)
				return
			}
			got[i] = out.Todo
		}()
	}
	wg.Wait()
	seen := map[string]string{}
	holders := []int{}
	for i, td := range got {
		if td == nil {
			continue
		}
		if other, dup := seen[td.ID]; dup {
			t.Fatalf("%s pulled by %s and %s", td.ID, other, workers[i].ID)
		}
		seen[td.ID] = workers[i].ID
		holders = append(holders, i)
	}
	if len(seen) != len(want) {
		t.Fatalf("pulled %v; want each of the %d to-dos once", seen, len(want))
	}

	// One held at a time: a worker holding one is given no second.
	w := holders[0]
	put(t, ctx, svc, api.Todo{Module: "shipping", Title: "one more"})
	if again, _ := svc.Pull(ctx, api.PullInput{By: workers[w]}); again.Todo != nil {
		t.Errorf("%s holds %s and pulled %s too", workers[w].ID, got[w].ID, again.Todo.ID)
	}

	// Each reports; the one more goes to one of them; then nothing is left.
	for _, i := range holders {
		if _, err := svc.Report(ctx, api.ReportInput{ID: got[i].ID, Result: api.Result{Status: api.StatusSuccess, Summary: "done", By: workers[i]}}); err != nil {
			t.Fatal(err)
		}
	}
	last, _ := svc.Pull(ctx, api.PullInput{By: workers[w]})
	if last.Todo == nil {
		t.Fatalf("after its report, %s pulled nothing: %+v", workers[w].ID, last)
	}
	if _, err := svc.Report(ctx, api.ReportInput{ID: last.Todo.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "done", By: workers[w]}}); err != nil {
		t.Fatal(err)
	}
	for _, wk := range workers {
		if out, _ := svc.Pull(ctx, api.PullInput{By: wk}); out.Todo != nil || out.Nothing == "" {
			t.Errorf("%s with nothing left: %+v; want nothing, saying so", wk.ID, out)
		}
	}
}
