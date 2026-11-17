// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"log/slog"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	plancore "github.com/katyoti/katyoti-fwk/internal/extensions/planification/internal/core"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// wakePlanner tells whoever waits on the work's moves (the internal scheduler)
// that something moved: a transition — of a plan, or of what one depends on.
func wakePlanner(ctx context.Context, ev cwmapi.EventEnvelope) {
	if !strings.HasSuffix(ev.Topic, ".transitioned") {
		return
	}
	if bus, err := application.UseService[eventbus.EventBus](ctx); err == nil && bus != nil {
		_ = bus.Publish(engine.PlanChangedTopic(), ev.Topic)
	}
}

// followPlanMoves ends « À faire »'s kept read on every move of the work the
// scheduler announces (plan.changed: a to-do put, assigned, reported,
// released or archived, a module's mode): the queue is built from the
// scheduler's to-dos too, whose moves carry no CWM event
func followPlanMoves(app *application.Application) error {
	bus, err := application.AppUseService[eventbus.EventBus](app)
	if err != nil || bus == nil {
		slog.Debug("planification: no event bus in this binary, a to-do's move does not end « À faire »'s kept read", "err", err)
		return nil
	}
	bus.Subscribe(engine.PlanChangedTopic(), func(string, any) { plancore.QueueChanged() })
	return nil
}
