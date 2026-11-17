// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// A sidecar shows a panel with this package alone: it opens
// the panel with OpenPanel over its bus's Publish, and the progress
// reporting its work already does - progress.Emit(ctx, snapshot), the
// neutral live-progress contract - feeds the panel's handle once
// ReportTo installs it on the work's context. The sidecar needs no engine,
// no agent and no inbox (internal/test/arch's panels-api guard).

// Reporter is the progress.Emitter that feeds h: each snapshot the work
// emits goes to the panel - a Progress (or *Progress) as its progress, any
// other snapshot laid over the data of the node target (a merge op, RFC
// 7396: `{data: snapshot}`), dropped when target is empty. An event the
// handle refuses (the panel ended) or cannot send is logged, never the
// work's failure.
func (h *Handle) Reporter(ctx context.Context, target string) progress.Emitter {
	return reporter{ctx: ctx, h: h, target: target}
}

// ReportTo installs h's Reporter on ctx: the work run with the context it
// answers reports into the panel through its own progress.Emit calls.
func ReportTo(ctx context.Context, h *Handle, target string) context.Context {
	return progress.WithEmitter(ctx, h.Reporter(ctx, target))
}

type reporter struct {
	ctx    context.Context
	h      *Handle
	target string
}

func (r reporter) Emit(snapshot any) {
	var err error
	switch s := snapshot.(type) {
	case Progress:
		err = r.h.Progress(r.ctx, s.Label, s.Current, s.Total)
	case *Progress:
		if s == nil {
			return
		}
		err = r.h.Progress(r.ctx, s.Label, s.Current, s.Total)
	default:
		if r.target == "" || snapshot == nil {
			return
		}
		err = r.h.Patch(r.ctx, render.Op{Op: render.OpMerge, Target: r.target, Value: map[string]any{"data": snapshot}})
	}
	if err != nil {
		slog.Warn("panel: a progress report was not sent", "panel", r.h.ID(), "err", err)
	}
}
