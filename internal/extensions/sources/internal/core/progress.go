// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"log/slog"
	"unicode/utf8"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// SourceSync, SyncAll and Reindex are long methods: run from
// the action surface, each answers its run and its panel at once, and its
// steps reach the panel the run lends it through progress.Emit - a
// panel.Progress per step. Called in a context that carries no emitter (a
// timer, a deposit, a test) a step is a no-op.

// step reports where a long method stands: current of total (total 0, a step
// whose length is not known).
func step(ctx context.Context, label string, current, total int) {
	progress.Emit(ctx, panel.Progress{Label: label, Current: int64(current), Total: int64(total)})
}

// quiet is ctx with its progress going nowhere: a long method calling
// another (SyncAll's SourceSync) keeps the panel's bar its own. The panel the
// context carries is kept: a connector called below still reports into it.
func quiet(ctx context.Context) context.Context {
	return progress.WithEmitter(ctx, mute{})
}

type mute struct{}

func (mute) Emit(any) {}

// callContext is what a connector call carries beside its input: within a
// long method's run, the panel it was lent - a connector that is itself long
// then reports into that panel instead of starting a run of its own, and
// answers its value.
func callContext(ctx context.Context) map[string]any {
	if h, ok := panel.HandleFrom(ctx); ok {
		return map[string]any{"panel": h.ID()}
	}
	return nil
}

// sourceLabel names a source on its step: its name, else its id.
func sourceLabel(s api.SourceSummary) string {
	if s.Name != "" {
		return s.Name
	}
	return s.ID
}

// syncing publishes where a source's sync stands: a
// source.sync event raised on the source, which CWM publishes on the source's
// subject - unlike step, never muted, so a run syncing several sources shows a
// line per source. The why of a failure is cut so the payload stays whole
// within what a raised event carries. A source that cannot receive it is
// logged, never the sync's failure.
func syncing(ctx context.Context, cwm cwmapi.CWMService, p api.SourceSyncProgress) {
	payload, err := syncPayload(p)
	if err != nil {
		return
	}
	if _, err := cwm.EventRaise(ctx, cwmapi.EventRaiseInput{Type: api.EventSourceSync, ObjectID: p.Source, Payload: payload}); err != nil {
		slog.Warn("sources: sync progress not raised", "source", p.Source, "phase", p.Phase, "err", err)
	}
}

// syncPayload is p as JSON, its why cut to SyncErrorMax bytes and further
// while the whole is longer than a raised event carries.
func syncPayload(p api.SourceSyncProgress) ([]byte, error) {
	p.Error = cutBytes(p.Error, api.SyncErrorMax)
	payload, err := json.Marshal(p)
	for err == nil && len(payload) > cwmapi.NoteMax && p.Error != "" {
		p.Error = cutBytes(p.Error, len(p.Error)-(len(payload)-cwmapi.NoteMax))
		payload, err = json.Marshal(p)
	}
	return payload, err
}

// readEvery is how many items a sync of total reads between two of its items
// events: about twenty a sync at most, so a large source does not flood its
// followers.
func readEvery(total int) int {
	if n := total / 20; n > 1 {
		return n
	}
	return 1
}

// cutBytes is s cut to at most max bytes on a rune boundary, marked "…" when
// cut.
func cutBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max < len("…") {
		return ""
	}
	cut := max - len("…")
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
