// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// publishPanels sends a step's display panel events on their panel's out
// subject, in order - after the step's checkpoint, as the
// executor calls it. With no bus or no universe the events are dropped: a
// screen joining later reads the panel's snapshot.
func publishPanels(ctx context.Context, events []panel.Event) {
	bus, err := application.UseService[mbapi.MessageBus](ctx)
	if err != nil || bus == nil {
		return
	}
	env := application.Env()
	if env == nil {
		return
	}
	tenant, universe := cenv.ResolveTenantUniverse(env.HomeDir())
	if tenant == "" || universe == "" {
		return
	}
	sendPanels(ctx, func(ctx context.Context, subject string, payload []byte) error {
		return bus.Publish(ctx, subject, payload)
	}, tenant, universe, events)
}

// sendPanels publishes each event on its panel's out subject.
func sendPanels(ctx context.Context, publish panel.Publish, tenant, universe string, events []panel.Event) {
	for _, e := range events {
		payload, err := json.Marshal(e)
		if err != nil {
			slog.Warn("task: a panel event could not be encoded", "panel", e.Panel, "seq", e.Seq, "err", err)
			continue
		}
		pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		if err := publish(pctx, subjects.PanelOutSubject(tenant, universe, e.Panel), payload); err != nil {
			slog.Warn("task: a panel event did not go out", "panel", e.Panel, "seq", e.Seq, "err", err)
		}
		cancel()
	}
}
