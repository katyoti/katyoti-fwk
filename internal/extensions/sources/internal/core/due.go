// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// How long a source stays fresh in each re-reading mode.
const (
	dailyFresh  = 20 * time.Hour
	onOpenFresh = 10 * time.Minute
)

// SyncDue re-reads the carrier's own sources whose mode is the one named and
// that were not read within its freshness, then reindexes its knowledge base
func (s *Service) SyncDue(ctx context.Context, in api.SyncDueInput) (api.SyncDueOutput, error) {
	fresh := map[string]time.Duration{api.SyncDaily: dailyFresh, api.SyncOnOpen: onOpenFresh}[in.Mode]
	if fresh == 0 {
		return api.SyncDueOutput{}, fmt.Errorf("%w: sources.SyncDue: mode is daily or on-open, got %q", cwmapi.ErrInvalidInput, in.Mode)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.SyncDueOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	if _, err := carrierOf(ctx, cwm, in.ID); err != nil {
		return api.SyncDueOutput{}, err
	}
	srcs, err := sourcesOf(ctx, cwm, in.ID)
	if err != nil {
		return api.SyncDueOutput{}, err
	}
	out := api.SyncDueOutput{OK: true, Synced: []string{}}
	for _, id := range dueSources(ctx, cwm, srcs, in.Mode, fresh, time.Now()) {
		if _, err := s.SourceSync(ctx, api.SourceSyncInput{ID: id}); err != nil {
			out.Message += fmt.Sprintf(" %s : %v.", id, err)
			continue
		}
		out.Synced = append(out.Synced, id)
	}
	if len(out.Synced) > 0 {
		if r, err := s.Reindex(ctx, api.ReindexInput{ID: in.ID}); err == nil && !r.OK {
			out.Message += " " + r.Message
		}
	}
	out.Message = fmt.Sprintf("%d source(s) relue(s) (%s).", len(out.Synced), in.Mode) + out.Message
	return out, nil
}

// dueSources are the carrier's own sources in a mode not read within fresh.
func dueSources(ctx context.Context, cwm cwmapi.CWMService, srcs []api.SourceSummary, mode string, fresh time.Duration, now time.Time) []string {
	var out []string
	for _, src := range srcs {
		if src.Via != "child" {
			continue
		}
		f, _, err := sourceFields(ctx, cwm, src.ID)
		if err != nil || !api.SyncIn(f.Sync, mode) {
			continue
		}
		if t, err := time.Parse(time.RFC3339, f.LastSynced); err == nil && now.Sub(t) < fresh {
			continue
		}
		out = append(out, src.ID)
	}
	return out
}

// onOpen is what opening a carrier's « Sources » tab starts: its due on-open
// sources re-read in the background, and its daily timer kept while a source
// is daily.
func (s *Service) onOpen(ctx context.Context, cwm cwmapi.CWMService, id string, srcs []api.SourceSummary) {
	daily := false
	for _, src := range srcs {
		if f, _, err := sourceFields(ctx, cwm, src.ID); err == nil && src.Via == "child" && api.SyncIn(f.Sync, api.SyncDaily) {
			daily = true
		}
	}
	if daily {
		if err := ensureDailyTimer(ctx, cwm, id); err != nil {
			slog.Debug("sources: daily timer", "carrier", id, "err", err)
		}
	}
	if len(dueSources(ctx, cwm, srcs, api.SyncOnOpen, onOpenFresh, time.Now())) == 0 {
		return
	}
	bg := context.WithoutCancel(ctx)
	go func() {
		if _, err := s.SyncDue(bg, api.SyncDueInput{ID: id, Mode: api.SyncOnOpen}); err != nil {
			slog.Warn("sources: on-open re-reading", "carrier", id, "err", err)
		}
	}()
}

// ensureDailyTimer gives the carrier the timed capability's daily timer
// whose timer.elapsed runs SyncDue (the knowledge capability's on_timer).
func ensureDailyTimer(ctx context.Context, cwm cwmapi.CWMService, id string) error {
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: moduleOf(id), ID: id})
	if err != nil {
		return err
	}
	timers, _ := got.Object.Capabilities["timed"]["timers"].([]any)
	for _, t := range timers {
		if m, ok := t.(map[string]any); ok && m["name"] == api.DailyTimer {
			return nil
		}
	}
	if _, ok := got.Object.Capabilities["timed"]; !ok {
		if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: id, Capability: "timed"}); err != nil {
			return err
		}
		if got, err = cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: moduleOf(id), ID: id}); err != nil {
			return err
		}
	}
	obj := got.Object
	tc := map[string]any{}
	for k, v := range obj.Capabilities["timed"] {
		tc[k] = v
	}
	tc["timers"] = append(timers, map[string]any{"name": api.DailyTimer, "every": "24h"})
	obj.Capabilities["timed"] = tc
	_, err = cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: moduleOf(id), Object: obj, Revision: got.Revision})
	return err
}

// raiseChanged raises source.changed on the source's carriers - the module
// it sits under and those it feeds - naming the items read again with
// another content. A carrier that cannot receive it is
// logged, never a sync's failure.
func (s *Service) raiseChanged(ctx context.Context, cwm cwmapi.CWMService, sourceID string, items []string) {
	payload, err := json.Marshal(api.SourceChanged{Source: sourceID, Items: items})
	if err != nil {
		return
	}
	carriers := []string{moduleOf(sourceID)}
	if edges, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: sourceID, Kind: api.FeedsEdge, Direction: cwmapi.DirectionOut}); err == nil {
		for _, e := range edges.Edges {
			if e.From == sourceID && e.To != carriers[0] {
				carriers = append(carriers, e.To)
			}
		}
	}
	for _, c := range carriers {
		if _, err := cwm.EventRaise(ctx, cwmapi.EventRaiseInput{Type: api.EventSourceChanged, ObjectID: c, Payload: payload}); err != nil {
			slog.Warn("sources: raise source.changed", "carrier", c, "source", sourceID, "err", err)
		}
	}
}
