// SPDX-License-Identifier: Apache-2.0

// Package local wires the inbox service into a binary.
package local

import (
	"context"
	"log/slog"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/internal/approval"
	inboxcore "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/internal/humantask"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/internal/waiting"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// Extension publishes the InboxService (`<provider>:inbox/<Verb>` over KTSP —
// the GUI's Inbox tab calls it there). It needs a CWMService in the same
// process: every verb is an Object API call. Where CWM runs, the inbox reacts
// to a held catch-up (cwmapi.EventObserver): it files the catch-up's one card,
// whose approval releases the held per-object events.
func Extension() application.Extension {
	svc := publish.PublishSimpleService[api.InboxService](
		func(context.Context) (api.InboxService, error) { return inboxcore.New(), nil },
		publish.WithServiceDef(api.InboxServiceDef),
		publish.WithKTSPContainer(),
	)
	catchUp := cwmapi.ObserveEvents(cwmapi.EventObserver{Name: "inbox-catch-up", Handle: inboxcore.OnCaughtUp})
	return func(init *application.ApplicationInit) {
		svc(init)
		catchUp(init)
	}
}

// ChecksExtension publishes the guide's inbox checks (`<provider>:inbox-checks/<Verb>`):
// read-only methods a recipe names as its check. Same process as the inbox.
func ChecksExtension() application.Extension {
	return publish.PublishSimpleService[api.InboxChecks](
		func(context.Context) (api.InboxChecks, error) { return inboxcore.NewChecks(), nil },
		publish.WithServiceDef(api.InboxChecksServiceDef),
		publish.WithKTSPContainer(),
	)
}

// approvalServer is the approval page of this process: one, shared by the
// ApprovalService and the serving hook that starts it.
var approvalServer = sync.OnceValue(func() *approval.Server {
	ts := approval.NewTailscale()
	return approval.New(approval.DefaultPort, nil, ts.Origin)
})

// ApprovalExtension publishes the ApprovalService (ktw-agents:approval/<Verb>)
// and, in the process serving the universe, starts the approval page on
// 127.0.0.1 - the person's passkeys reached through iam's PasskeyService.
func ApprovalExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		publish.PublishSimpleService[api.ApprovalService](
			func(context.Context) (api.ApprovalService, error) {
				return approval.Service{S: approvalServer(), TS: approval.NewTailscale()}, nil
			},
			publish.WithServiceDef(api.ApprovalServiceDef),
			publish.WithKTSPContainer(),
		)(init)
		init.AddContribute("inbox", ".approval-page", func(app *application.Application) error {
			start := func(ctx context.Context) {
				s := approvalServer()
				if pk, err := application.UseService[profileapi.PasskeyService](ctx); err == nil {
					s.Passkeys = pk
				}
				l, err := s.Listen()
				if err != nil {
					slog.Warn("inbox: the approval page cannot listen", "port", s.Port, "err", err)
					return
				}
				if err := s.Serve(ctx, l); err != nil {
					slog.Warn("inbox: the approval page stopped", "err", err)
				}
			}
			if err := application.Contribute(app, ktsapi.ServingHook{Name: "approval-page", Start: start}); err != nil {
				slog.Debug("inbox: no serving hooks in this binary, no approval page", "err", err)
			}
			return nil
		})
	}
}

// HumanTaskExtension registers the human task: the `human-task`
// machine strategy any asker starts, and the inbox's collector of its to-dos -
// the answer kept on the to-do and the waiting run told. Same process as the
// scheduler and the task engine.
func HumanTaskExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("inbox", ".human-task", func(app *application.Application) error {
			if err := strategies.ContributeStrategy(app, humantask.MachineStrategyDef); err != nil {
				return err
			}
			// A question asked from a chat shows there as « À faire »'s card
			// : the waiting list draws it.
			humantask.CardOf = waiting.ChatCard
			return contributeAnswers(app)
		})
	}
}

// AnswersExtension registers the human task's side the scheduler reads, its
// machine strategy left out: the inbox's collector of its to-dos (the answer
// kept, its run told) and its holding lists' members. A binary or a test with
// no task engine registers it alone.
func AnswersExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("inbox", ".human-task-answers", contributeAnswers)
	}
}

func contributeAnswers(app *application.Application) error {
	if err := application.Contribute(app, schedapi.Collector{Owner: api.Owner, Collect: humantask.Collect}); err != nil {
		return err
	}
	// Who may pull a human task's to-do: its holding list's members.
	return application.Contribute(app, humantask.GroupMembers)
}

// WaitingExtension publishes « À faire », the one place:
// everything waiting on a person (`<provider>:waiting/<Verb>`), and opens the
// point where the askers that do not yet ask through a human task contribute
// their questions (api.WaitingSource). Same process as the inbox, the
// scheduler and the CWM.
func WaitingExtension() application.Extension {
	svc := publish.PublishSimpleService[api.WaitingService](
		func(context.Context) (api.WaitingService, error) { return waiting.New(), nil },
		publish.WithServiceDef(api.WaitingServiceDef),
		publish.WithKTSPContainer(),
	)
	return func(init *application.ApplicationInit) {
		init.AddRegisterPoints("inbox", ".waiting-sources", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[api.WaitingSource]())
		})
		svc(init)
	}
}
