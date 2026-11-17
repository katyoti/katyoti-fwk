// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// ObjectTree answers the subtree's size under the root.
func (c *treeCWM) ObjectTree(_ context.Context, in cwmapi.ObjectTreeInput) (cwmapi.ObjectTreeOutput, error) {
	n := 0
	var count func(id string)
	count = func(id string) {
		n++
		for _, o := range c.objects {
			if o.Parent == id {
				count(o.ID)
			}
		}
	}
	count(in.Root)
	return cwmapi.ObjectTreeOutput{Total: n}, nil
}

type recordEmitter struct{ got []any }

func (e *recordEmitter) Emit(s any) { e.got = append(e.got, s) }

// : Verify is declared long, and reports each check into
// the panel it runs under as the check answers - one progress per check,
// naming the object, the check and its status, counted over the subtree's
// objects. Every check runs: a fresh verdict counts only for verify().
func TestVerifyReportsEachCheckAsItAnswers(t *testing.T) {
	if !api.VerificationServiceDef.Methods["Verify"].LongRunning {
		t.Error("Verify is not declared long: run from the action surface it would block its caller instead of answering its run and its panel")
	}
	if api.VerificationServiceDef.Methods["VerifyFunction"].LongRunning {
		t.Error("VerifyFunction is declared long, but a gate asks verify() and waits for its answer")
	}

	now := time.Now()
	cwm := &treeCWM{objects: []cwmapi.Object{
		{ID: "m.F-1", Type: "feature"},
		criterion("m.AC-1", "bash live.sh"), verdict("m.AC-1", "bash live.sh", "pass", now.Add(-time.Minute)),
		criterion("m.AC-2", "go test ./x"),
	}}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		application.DeclareSimpleService[cwmapi.CWMService](func(context.Context) (cwmapi.CWMService, error) { return cwm, nil }))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	em := &recordEmitter{}
	ctx := progress.WithEmitter(application.WithApplication(context.Background(), app), em)

	out, err := New().Verify(ctx, api.VerifyInput{ID: "m.F-1"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Runs != 2 || out.Passed != 2 {
		t.Fatalf("Verify ran %d checks, %d passed: want both criteria run, the fresh verdict not used", out.Runs, out.Passed)
	}
	if len(em.got) != 2 {
		t.Fatalf("got %d reports, want one per check: %+v", len(em.got), em.got)
	}
	for i, want := range []string{"m.AC-1", "m.AC-2"} {
		p, ok := em.got[i].(panel.Progress)
		if !ok {
			t.Fatalf("report %d is %T, want a panel.Progress", i, em.got[i])
		}
		if !strings.HasPrefix(p.Label, want+" · ") || !strings.HasSuffix(p.Label, ": pass") {
			t.Errorf("report %d labelled %q: want the object %s, its check and its status", i, p.Label, want)
		}
		if p.Total != 4 || p.Current < 1 || p.Current > p.Total {
			t.Errorf("report %d counts %d of %d: want the walked objects of the subtree's 4", i, p.Current, p.Total)
		}
	}

	// No reporter installed (a gate, a CLI call): the same answer, nothing sent.
	plain, err := New().Verify(application.WithApplication(context.Background(), app), api.VerifyInput{ID: "m.F-1"})
	if err != nil || plain.Runs != out.Runs || plain.Passed != out.Passed {
		t.Errorf("without a reporter Verify answers %+v (%v), want %+v", plain, err, out)
	}
}
