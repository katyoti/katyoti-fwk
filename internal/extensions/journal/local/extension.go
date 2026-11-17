// SPDX-License-Identifier: Apache-2.0

// Package local wires the journal into a binary: the JournalService
// (`<provider>:journal/<Verb>`), and the CWM event observer that records — in
// the process that holds the workspace, so the journal has one writer.
package local

import (
	"context"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	journalcore "github.com/katyoti/katyoti-fwk/internal/extensions/journal/internal/core"
)

// recorder is what the observer calls on the journal service.
type recorder interface {
	Record(ctx context.Context, ev cwmapi.EventEnvelope) error
}

// Extension publishes the JournalService and contributes its CWM observer. It
// needs a CWMService in the same process: every read and write is an Object
// API call.
func Extension() application.Extension {
	service := publish.PublishService[api.JournalService, journalcore.JournalServiceConfig](
		func(_ context.Context, cfg *journalcore.JournalServiceConfig) (api.JournalService, error) {
			return journalcore.New(cfg.RetentionDays), nil
		},
		publish.WithServiceDef(api.JournalServiceDef),
		publish.WithKTSPContainer(),
	)
	observer := cwmapi.ObserveEvents(cwmapi.EventObserver{
		Name: "journal",
		Handle: func(ctx context.Context, ev cwmapi.EventEnvelope) {
			svc, err := application.UseService[api.JournalService](ctx)
			if err != nil {
				slog.Warn("journal: service unavailable; event not recorded", "topic", ev.Topic, "error", err)
				return
			}
			r, ok := svc.(recorder)
			if !ok {
				return // a remote journal: this process does not record
			}
			if err := r.Record(ctx, ev); err != nil {
				slog.Warn("journal: record failed", "topic", ev.Topic, "error", err)
			}
		},
	})
	return func(init *application.ApplicationInit) {
		service(init)
		observer(init)
	}
}
