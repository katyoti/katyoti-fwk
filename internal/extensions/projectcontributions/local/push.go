// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/internal/core"
)

// PushExtension is the publisher of object-backed entries (decision D1), in
// the process hosting CWM: each object event pushes the object's entry, or its
// removal, on api.PushSubject; the first event also pushes a snapshot and
// starts answering the hub's snapshot requests (api.SnapshotRequestSubjectAll)
// and looking at the module files the entries' layers read (a write of one
// pushes its object again).
// With no message bus (a process without a hub) it pushes nothing.
func PushExtension() application.Extension {
	return PushExtensionTo(busSend)
}

// PushExtensionTo is PushExtension sending through send (a test records the
// pushes). The snapshot requests are answered only where a message bus is.
func PushExtensionTo(send func(ctx context.Context, p api.EntriesPush) error) application.Extension {
	var pusher *core.Pusher
	pusher = core.NewPusher(send, func(ctx context.Context) {
		if err := pusher.Snapshot(ctx, ""); err != nil {
			slog.Warn("project-contributions: the first snapshot was not pushed", "error", err)
		}
		answerSnapshotRequests(ctx, pusher)
		go pusher.WatchLayerFiles(ctx, layerFilesInterval)
	})
	return cwmapi.ObserveEvents(cwmapi.EventObserver{Name: "project-contributions-push", Handle: pusher.OnEvent})
}

// layerFilesInterval is how often the module files the pushed entries'
// layers read are looked at: a write of one publishes no object event, so
// its object is pushed again within this delay.
var layerFilesInterval = 2 * time.Second

// busSend publishes p on its project's push subject.
func busSend(ctx context.Context, p api.EntriesPush) error {
	bus, err := application.UseService[mbapi.MessageBus](ctx)
	if err != nil || bus == nil {
		return nil // no hub: nothing serves the entries
	}
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tenant, universe := cenv.ResolveTenantUniverse(application.Env().HomeDir())
	return bus.Publish(ctx, api.PushSubject(tenant, universe, p.Token), body)
}

// answerSnapshotRequests answers each hub's snapshot request with the asked
// project's snapshot, for as long as ctx (the application's) lives.
func answerSnapshotRequests(ctx context.Context, pusher *core.Pusher) {
	bus, err := application.UseService[mbapi.MessageBus](ctx)
	if err != nil || bus == nil {
		return
	}
	tenant, universe := cenv.ResolveTenantUniverse(application.Env().HomeDir())
	_, err = bus.Subscribe(ctx, api.SnapshotRequestSubjectAll(tenant, universe), func(_ context.Context, msg mbapi.Message) {
		parts := strings.Split(msg.Subject(), ".")
		if len(parts) < 2 {
			return
		}
		if err := pusher.Snapshot(ctx, parts[len(parts)-2]); err != nil {
			slog.Warn("project-contributions: a requested snapshot was not pushed", "subject", msg.Subject(), "error", err)
		}
	})
	if err != nil {
		slog.Warn("project-contributions: snapshot requests are not answered", "error", err)
	}
}
