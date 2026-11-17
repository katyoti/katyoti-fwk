// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	eventscore "github.com/katyoti/katyoti-fwk/internal/extensions/events/internal/core"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
)

// ReactionsExtension connects CWM's asynchronous subscriptions to the
// scheduler: it takes CWM's matches (cwmapi.TakeMatches)
// runs the reaction to-dos as their owner (schedlocal.ContributeRunner),
// collects them (schedlocal.ContributeCollector), and publishes the reactions
// verbs (ktw-agents:reactions/<Verb>). It sits above both: CWM imports nothing
// of it, and it writes CWM state through CWM's verbs only. It needs the CWM
// service, the scheduler and the ActionService in the same process.
func ReactionsExtension() application.Extension { return ReactionsExtensionWith(ReactionsOptions{}) }

// ReactionsOptions tunes the reactions part (a test's short backoff).
type ReactionsOptions struct {
	// BackoffSeconds before a failed reaction is tried again; 0: 30.
	BackoffSeconds int
	// SourcesDir is where the sources keep their cursors, for the Events read
	// ("": the data folder's events/).
	SourcesDir string
}

// ReactionsExtensionWith is ReactionsExtension with options.
func ReactionsExtensionWith(o ReactionsOptions) application.Extension {
	r := eventscore.NewReactions(o.BackoffSeconds).WithSourcesDir(o.SourcesDir)
	service := publish.PublishSimpleService[eventsapi.ReactionService](
		func(context.Context) (eventsapi.ReactionService, error) { return r, nil },
		publish.WithServiceDef(eventsapi.ReactionServiceDef),
		publish.WithKTSPContainer(),
	)
	taker := cwmapi.TakeMatches(cwmapi.MatchTaker{Name: eventsapi.Owner, Take: r.Take})
	return func(init *application.ApplicationInit) {
		service(init)
		taker(init)
		init.AddContribute("events", ".reactions", func(app *application.Application) error {
			if err := schedlocal.ContributeRunner(app, schedapi.Runner{Owner: eventsapi.Owner, Run: r.RunTodo}); err != nil {
				return err
			}
			return schedlocal.ContributeCollector(app, schedapi.Collector{Owner: eventsapi.Owner, Collect: r.Collect})
		})
	}
}
