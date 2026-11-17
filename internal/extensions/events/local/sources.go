// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"log/slog"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	eventscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/events/contrib"
	eventscore "github.com/katyoti/katyoti-fwk/internal/extensions/events/internal/core"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// SourcesOptions tunes the event sources: where their cursors live, the
// repository read, the clock, how often the serving process polls (tests).
type SourcesOptions struct {
	StateDir string
	Repo     string
	Now      func() time.Time
	// Interval between two polls of the serving process; 0: 10 s. Negative:
	// no loop (a test steps Poll itself).
	Interval time.Duration
}

// SourcesExtension raises the events that are not object writes
// : it contributes the events pack (code.changed
// timer.elapsed, the `timed` capability), publishes the event sources
// (ktw-agents:event-sources/<Verb>: Poll, Raise, After), and, in the process
// that serves the universe, polls them — a new git HEAD, the timers due, the
// delayed events due. It needs the CWM service and the scheduler in the same
// process.
func SourcesExtension() application.Extension { return SourcesExtensionWith(SourcesOptions{}) }

// SourcesExtensionWith is SourcesExtension with options.
func SourcesExtensionWith(o SourcesOptions) application.Extension {
	s := eventscore.NewSources(eventscore.SourcesOptions{StateDir: o.StateDir, Repo: o.Repo, Now: o.Now})
	service := publish.PublishSimpleService[eventsapi.EventSourceService](
		func(context.Context) (eventsapi.EventSourceService, error) { return s, nil },
		publish.WithServiceDef(eventsapi.EventSourceServiceDef),
		publish.WithKTSPContainer(),
	)
	pack := eventscontrib.Pack("")
	interval := o.Interval
	if interval == 0 {
		interval = 10 * time.Second
	}
	return func(init *application.ApplicationInit) {
		pack(init)
		service(init)
		if interval < 0 {
			return
		}
		init.AddContribute("events", ".sources-loop", func(app *application.Application) error {
			if err := application.Contribute(app, ktsapi.ServingHook{Name: "event-sources", Start: func(ctx context.Context) {
				t := time.NewTicker(interval)
				defer t.Stop()
				for {
					if _, err := s.Poll(ctx, eventsapi.PollInput{}); err != nil {
						slog.Warn("events: a poll of the sources", "error", err)
					}
					select {
					case <-ctx.Done():
						return
					case <-t.C:
					}
				}
			}}); err != nil {
				slog.Debug("events: no serving hooks in this binary - the sources are polled on demand only", "err", err)
			}
			return nil
		})
	}
}
