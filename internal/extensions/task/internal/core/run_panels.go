// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A RUN'S PANELS END WITH IT - every panel a run opens carries
// its run id as owner. However the run ends - completed, failed, cancelled by
// a person's Stop, killed, interrupted by a restart - each panel it owns that
// is neither complete nor closed is completed at its next sequence, with how
// the run ended: no panel stays « en cours » once no run drives it, and one
// left asking would wait for an answer no run takes. A session's chat ends
// with its conversation: while a run above its owner lives (a workflow whose
// first agent mounted the chat), it stays open. The serving process sweeps
// the universe's panels too, at start and with each restore sweep: a panel
// whose owner run ended - in a process that died, or that no record knows
// any more - is completed then.

// PanelStatusInterrupted is the status a panel of a run a restart ended
// completes with.
const PanelStatusInterrupted = "interrupted"

// PanelStatusRunGone is the status a panel completes with when no record
// knows its owner run any more.
const PanelStatusRunGone = "ended"

// endedPanelStatus is the status the panels of a run that ended, as told by
// t, complete with.
func endedPanelStatus(t engine.RunTransition) string {
	switch t {
	case engine.RunCompleted:
		return string(taskapi.RunStatusCompleted)
	case engine.RunFailed:
		return string(taskapi.RunStatusFailed)
	case engine.RunCancelled:
		return string(taskapi.RunStatusCancelled)
	case engine.RunKilled:
		return string(taskapi.RunStatusKilled)
	}
	return PanelStatusInterrupted
}

// Seams to the panel server and the panel subjects - package-level so tests
// substitute a fake (a service's own panelSeams first).
var (
	resolveLeftPanels = func(ctx context.Context) (panelapi.PanelService, error) {
		return application.UseService[panelapi.PanelService](ctx)
	}
	sendLeftPanels = publishPanels
)

// panelSeams is the panel server a service's runs' panels are listed on and
// how their completes are sent.
type panelSeams struct {
	resolve func(context.Context) (panelapi.PanelService, error)
	send    func(context.Context, []panel.Event)
}

// panelSeam is the service's panel seams, the package's when it has none.
func (s *Service) panelSeam() panelSeams {
	if s.panels != nil {
		return *s.panels
	}
	return panelSeams{resolve: resolveLeftPanels, send: sendLeftPanels}
}

// endPanels completes the panels of a run that ended (told as t). Called
// once its end is set in the registry.
func (s *Service) endPanels(runID string, t engine.RunTransition) {
	ctx := s.appCtx
	if ctx == nil {
		ctx = context.Background()
	}
	runs := s.runsKnown()
	status := endedPanelStatus(t)
	completePanels(ctx, s.panelSeam(), panelapi.ListInput{Owner: runID}, func(p panelapi.Summary) (string, bool) {
		if p.Owner != runID || !runs.panelEnded(p) {
			return "", false
		}
		return status, true
	})
}

// sweepPanels completes every panel of the universe whose owner run ended
// or that no record knows: what a process that died left
// open, what an end whose completion did not go out left.
func (s *Service) sweepPanels(ctx context.Context) {
	runs := s.runsKnown()
	completePanels(ctx, s.panelSeam(), panelapi.ListInput{}, func(p panelapi.Summary) (string, bool) {
		if !runs.panelEnded(p) {
			return "", false
		}
		return runs.endStatus(p.Owner), true
	})
}

// completePanels completes, at its next sequence, each panel the server
// lists for in that is neither complete nor closed and that ended says has
// ended, with the status it answers. No panel server reached: nothing is
// sent (the run's end is told all the same).
func completePanels(ctx context.Context, seam panelSeams, in panelapi.ListInput, ended func(panelapi.Summary) (string, bool)) {
	svc, err := seam.resolve(ctx)
	if err != nil || svc == nil {
		return
	}
	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	out, err := svc.List(lctx, in)
	if err != nil {
		slog.Warn("tasks: the panels of ended runs could not be listed - they stay open", "owner", in.Owner, "err", err)
		return
	}
	var events []panel.Event
	for _, p := range out.Panels {
		if p.State == panel.StateComplete || p.State == panel.StateClosed {
			continue
		}
		status, ok := ended(p)
		if !ok {
			continue
		}
		tr := panel.Track{ID: p.Panel, Seq: p.Seq, State: p.State}
		e, err := tr.Next(panel.Event{Kind: panel.KindComplete, Complete: &panel.Complete{Status: status}})
		if err != nil {
			slog.Warn("tasks: a panel of an ended run could not be completed", "owner", p.Owner, "panel", p.Panel, "err", err)
			continue
		}
		events = append(events, e)
	}
	if len(events) > 0 {
		seam.send(ctx, events)
	}
}

// knownRuns answers what the process knows of a run: its registry, the
// ended runs it records, the manifests on disk (every process of the
// universe writes its runs there).
type knownRuns struct {
	s     *Service
	ended map[string]taskapi.RunInfo // read once, on first need
}

func (s *Service) runsKnown() *knownRuns { return &knownRuns{s: s} }

// runRecord is what is known of one run.
type runRecord struct {
	parent string
	status taskapi.RunStatus
	live   bool
	known  bool
}

func (k *knownRuns) of(id string) runRecord {
	if info, _, ok := k.s.reg.snapshot(id); ok {
		return runRecord{parent: info.ParentRunID, status: info.Status, live: !terminal(info.Status), known: true}
	}
	if k.s.runs == nil {
		return runRecord{}
	}
	if k.ended == nil {
		k.ended = map[string]taskapi.RunInfo{}
		for _, info := range k.s.runs.endedRuns() {
			k.ended[info.RunID] = info
		}
	}
	if info, ok := k.ended[id]; ok {
		return runRecord{parent: info.ParentRunID, status: info.Status, known: true}
	}
	// On disk and not ended: another process's run, or one a restart takes up.
	if m, err := k.s.runs.readManifest(id); err == nil && m.RunID == id {
		return runRecord{parent: m.Request.ParentRunID, status: m.Ended, live: m.Ended == "", known: true}
	}
	return runRecord{}
}

// panelEnded says whether no run drives a panel any more: its owner is a run
// that no longer lives - and for a chat, no run above it lives either. A
// panel no run owns (a screen's own) is never ended here.
func (k *knownRuns) panelEnded(p panelapi.Summary) bool {
	if !strings.HasPrefix(p.Owner, "run-") {
		return false
	}
	r := k.of(p.Owner)
	if r.live {
		return false
	}
	if p.Type != panel.TypeChat {
		return true
	}
	seen := map[string]bool{p.Owner: true}
	for id := r.parent; id != "" && !seen[id]; {
		seen[id] = true
		above := k.of(id)
		if above.live {
			return false
		}
		id = above.parent
	}
	return true
}

// endStatus is how a panel's owner run ended, as its panel completes.
func (k *knownRuns) endStatus(id string) string {
	if r := k.of(id); r.known && terminal(r.status) {
		return string(r.status)
	}
	return PanelStatusRunGone
}
