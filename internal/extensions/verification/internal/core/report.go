// SPDX-License-Identifier: Apache-2.0

package core

// A VERIFICATION REPORTS EACH CHECK. Verify is declared
// long: run from the action surface it is a method run that owns a panel,
// and the walk reports each check's answer into it as the check answers -
// the object and the check, its status, how many of the subtree's objects
// are walked - through the neutral progress contract (progress.Emit with a
// panel.Progress). With no reporter installed (a gate, a CLI call) nothing is
// counted and nothing is sent.

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// checkReporter emits one progress report per check answered.
type checkReporter struct {
	ctx    context.Context
	on     bool
	walked int64
	total  int64
}

// newCheckReporter reports into the emitter ctx carries; total is the
// subtree's size under root, read only when there is one to report to.
func newCheckReporter(ctx context.Context, cwm cwmapi.CWMService, root string) *checkReporter {
	r := &checkReporter{ctx: ctx}
	if _, ok := progress.EmitterFrom(ctx); !ok {
		return r
	}
	r.on = true
	if tr, err := cwm.ObjectTree(ctx, cwmapi.ObjectTreeInput{Root: root}); err == nil {
		r.total = int64(tr.Total)
	}
	return r
}

// object counts one walked object and reports each of its checks' answers.
func (r *checkReporter) object(runs []cwmapi.HookRun) {
	if !r.on {
		return
	}
	r.walked++
	total := r.total
	if total < r.walked {
		total = 0 // the tree undercounted: an open bar rather than a wrong one
	}
	for _, run := range runs {
		o := verifyOutcome(run)
		progress.Emit(r.ctx, panel.Progress{Label: checkLabel(o), Current: r.walked, Total: total})
	}
}

// checkLabel names a check and its answer: "<object> · <check>: <status>".
func checkLabel(o api.VerifyOutcome) string {
	return o.ObjectID + " · " + o.URI + ": " + o.Status
}
