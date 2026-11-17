// SPDX-License-Identifier: Apache-2.0

// Package local wires the planification extension into a binary: the
// RequestService (`<provider>:requests/<Verb>`, CLI verbs for free), the
// WorkService (`<provider>:work/<Verb>`), the TriggerService
// (`<provider>:triggers/<Verb>`), the PlanService (`<provider>:plans/<Verb>`), the
// ReleaseService (`<provider>:releases/<Verb>`), the ChangeService
// (`<provider>:changes/<Verb>`: the change log as data), the PlanningService
// (`<provider>:planning/<Verb>`), the ViewService (`<provider>:views/<Verb>`), the
// PublicationService (`<provider>:publications/<Verb>`), the CWM observer — in
// the process that holds the workspace: it wakes the scheduler on a transition,
// follows the gates, plans an answered question's intake again, reports a
// person's to-do from their inbox answer, ends what a request's end ends — and,
// for a binary running the agent loop, the `file-request` chat tool.
package local

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"weak"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	plancore "github.com/katyoti/katyoti-fwk/internal/extensions/planification/internal/core"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// waker is what the observer calls on the request service.
type waker interface {
	Answered(ctx context.Context, entry string) ([]string, error)
}

// gater is the request service's side of named gates (core/gates.go): a
// refused move plans its resolution, and what resolves it moves it on.
type gater interface {
	GateRefused(ctx context.Context, p cwmapi.ObjectTransitionRefusedEventPayload) error
	GateRequestEnded(ctx context.Context, request string) error
	GateAnswerKept(ctx context.Context, answer string) (bool, error)
	// GateAnswered follows an answered inbox validation of a gate, asked
	// while IN-039's switch is off.
	GateAnswered(ctx context.Context, entry string) (bool, error)
	// IntakeAnswerKept takes a kept answer to a request's intake question
	//
	IntakeAnswerKept(ctx context.Context, answer string) (bool, error)
	GateMoved(ctx context.Context, id, to, transition string) error
}

// gates is the in-process gater; false for a remote service.
func gates(ctx context.Context) (gater, bool) {
	svc, err := application.UseService[api.RequestService](ctx)
	if err != nil {
		return nil, false
	}
	g, ok := svc.(gater)
	return g, ok
}

// Extension publishes the RequestService, the ReleaseService, the
// PlanningService, the ViewService, the PublicationService and the
// PlanService, and contributes the observer that
// wakes a request parked on its dependencies when the last of them ends. It
// needs a CWMService in the same process: every read and write is an Object
// API call.
func Extension() application.Extension {
	service := publish.PublishSimpleService[api.RequestService](
		func(context.Context) (api.RequestService, error) { return plancore.New(), nil },
		publish.WithServiceDef(api.RequestServiceDef),
		publish.WithKTSPContainer(),
	)
	releases := publish.PublishSimpleService[api.ReleaseService](
		func(context.Context) (api.ReleaseService, error) { return plancore.NewReleases(), nil },
		publish.WithServiceDef(api.ReleaseServiceDef),
		publish.WithKTSPContainer(),
	)
	changes := publish.PublishSimpleService[api.ChangeService](
		func(context.Context) (api.ChangeService, error) { return plancore.NewChanges(), nil },
		publish.WithServiceDef(api.ChangeServiceDef),
		publish.WithKTSPContainer(),
	)
	increments := publish.PublishSimpleService[api.IncrementService](
		func(context.Context) (api.IncrementService, error) { return plancore.NewIncrements(), nil },
		publish.WithServiceDef(api.IncrementServiceDef),
		publish.WithKTSPContainer(),
	)
	streams := publish.PublishSimpleService[api.StreamService](
		func(context.Context) (api.StreamService, error) { return plancore.NewStreams(), nil },
		publish.WithServiceDef(api.StreamServiceDef),
		publish.WithKTSPContainer(),
	)
	planning := publish.PublishSimpleService[api.PlanningService](
		func(context.Context) (api.PlanningService, error) { return plancore.NewPlanning(), nil },
		publish.WithServiceDef(api.PlanningServiceDef),
		publish.WithKTSPContainer(),
	)
	views := publish.PublishSimpleService[api.ViewService](
		func(context.Context) (api.ViewService, error) { return plancore.NewViews(), nil },
		publish.WithServiceDef(api.ViewServiceDef),
		publish.WithKTSPContainer(),
	)
	publications := publish.PublishSimpleService[api.PublicationService](
		func(context.Context) (api.PublicationService, error) { return plancore.NewPublications(), nil },
		publish.WithServiceDef(api.PublicationServiceDef),
		publish.WithKTSPContainer(),
	)
	plans := publish.PublishSimpleService[api.PlanService](
		func(context.Context) (api.PlanService, error) { return plancore.NewPlans(), nil },
		publish.WithServiceDef(api.PlanServiceDef),
		publish.WithKTSPContainer(),
	)
	work := publish.PublishSimpleService[api.WorkService](
		func(context.Context) (api.WorkService, error) { return plancore.NewWork(), nil },
		publish.WithServiceDef(api.WorkServiceDef),
		publish.WithKTSPContainer(),
	)
	triggers := publish.PublishSimpleService[api.TriggerService](
		func(context.Context) (api.TriggerService, error) { return plancore.NewTriggers(), nil },
		publish.WithServiceDef(api.TriggerServiceDef),
		publish.WithKTSPContainer(),
	)
	observer := cwmapi.ObserveEvents(cwmapi.EventObserver{
		Name:   "planification-wake",
		Handle: handle,
	})
	return func(init *application.ApplicationInit) {
		service(init)
		releases(init)
		changes(init)
		increments(init)
		streams(init)
		planning(init)
		views(init)
		publications(init)
		work(init)
		// planification owns the to-dos it puts: it collects their results.
		init.AddContribute("planification", ".collector", func(app *application.Application) error {
			return schedlocal.ContributeCollector(app, schedapi.Collector{Owner: api.Owner, Collect: plancore.Collect})
		})
		// A pull's waiting line of its to-dos starts with the kind and who
		// moves it, as planning check names that wait.
		init.AddContribute("planification", ".wait-namer", func(app *application.Application) error {
			return schedlocal.ContributeWaitNamer(app, schedapi.WaitNamer{Owner: api.Owner, Name: plancore.NameWait})
		})
		// Its questions in « À faire », the inbox's one list
		// until they ask through human tasks.
		init.AddContribute("planification", ".waiting", func(app *application.Application) error {
			if err := application.Contribute(app, plancore.WaitingSource()); err != nil {
				slog.Debug("planification: no « À faire » in this binary, its questions are not listed there", "err", err)
			}
			return nil
		})
		// The reconciler sweeps in the serving process only - the one holding
		// the CWM lease; a one-shot command never runs it.
		init.AddContribute("planification", ".reconciler", func(app *application.Application) error {
			if err := application.Contribute(app, ktsapi.ServingHook{Name: "planification-reconciler", Start: plancore.RunReconciler}); err != nil {
				slog.Debug("planification: no serving hooks in this binary, the reconciler does not sweep", "err", err)
			}
			return nil
		})
		// « À faire » is built from the scheduler's to-dos too: their moves
		// end its kept read as an object's change does.
		init.AddBootstrap("planification", ".queue-changes", followPlanMoves)
		triggers(init)
		plans(init)
		observer(init)
	}
}

// republisher is what the observer calls on the publication service.
type republisher interface {
	OnReleased(ctx context.Context, release string) ([]string, error)
}

// handle marks the universe changed for « À faire »'s kept read, runs the
// planning loop on every change to a request, takes back a
// blocked job, wakes the dependents of a network node (a request, a step) that just
// ended, and republishes what a released release makes out of date.
func handle(ctx context.Context, ev cwmapi.EventEnvelope) {
	// A change: « À faire »'s kept read answers no click any more.
	plancore.QueueChanged()
	act := activityOf(ctx)
	act.inflight.Add(1)
	defer wakePlanner(ctx, ev)
	defer func() {
		act.last.Store(time.Now().UnixNano())
		act.inflight.Add(-1)
	}()
	switch {
	case strings.HasSuffix(ev.Topic, ".created") || strings.HasSuffix(ev.Topic, ".updated"):
		// CWM's own payload type: a field it renames breaks the build here,
		// never silently the loop.
		var w cwmapi.ObjectEventPayload
		if json.Unmarshal(ev.Payload, &w) != nil || w.ObjectID == "" {
			return
		}
		defer act.handled.Store(w.ObjectID, w.Revision)
		if w.Type == inboxapi.RequestType {
			if g, ok := gates(ctx); ok {
				if mine, err := g.GateAnswered(ctx, w.ObjectID); mine {
					if err != nil {
						slog.Warn("planification: a gate validation's answer was not followed", "question", w.ObjectID, "error", err)
					}
					return
				}
			}
			answered(ctx, w.ObjectID)
			return
		}
		// A kept answer may confirm a gate's requirement: the
		// object waiting on it moves on.
		if w.Type == inboxapi.AnswerType && w.Action == "created" {
			if g, ok := gates(ctx); ok {
				if _, err := g.GateAnswerKept(ctx, w.ObjectID); err != nil {
					slog.Warn("planification: a gate confirmation's answer was not followed", "answer", w.ObjectID, "error", err)
				}
				// Or answer a request's intake question.
				if _, err := g.IntakeAnswerKept(ctx, w.ObjectID); err != nil {
					slog.Warn("planification: an intake question's answer was not taken", "answer", w.ObjectID, "error", err)
				}
			}
			return
		}
		// A criterion's verdict may complete a realized feature's proof.
		if w.Type == "verdict" {
			plancore.VerdictRecorded(ctx, w.ObjectID)
			return
		}
		// An increment or a release changed past its service (its release
		// edited by hand): the work labelled with it is labelled again.
		if w.Action == "updated" && (w.Type == api.IncrementType || w.Type == api.ReleaseType) {
			if err := plancore.SettleScope(ctx, w.ObjectID); err != nil {
				slog.Warn("planification: the work labelled with a changed scope was not labelled again", "scope", w.ObjectID, "error", err)
			}
		}
		// A depends-on edge removed by hand releases the work it held.
		if w.Action == "updated" {
			if err := plancore.SettleStaleWaits(ctx, w.ObjectID); err != nil {
				slog.Warn("planification: the work a removed dependency held was not released", "request", w.ObjectID, "error", err)
			}
		}
		return
	case strings.HasSuffix(ev.Topic, ".moved"):
		// An increment or a release moved to a new id: its work, still
		// labelled with the old one, follows it.
		var p cwmapi.ObjectMovedEventPayload
		if json.Unmarshal(ev.Payload, &p) != nil || p.FromID == "" || p.FromID == p.ObjectID {
			return
		}
		if p.Type == api.IncrementType || p.Type == api.ReleaseType {
			if err := plancore.SettleScope(ctx, p.FromID); err != nil {
				slog.Warn("planification: the work of a moved scope was not labelled again", "from", p.FromID, "to", p.ObjectID, "error", err)
			}
		}
		return
	case strings.HasSuffix(ev.Topic, ".transition-refused"):
		var p cwmapi.ObjectTransitionRefusedEventPayload
		if json.Unmarshal(ev.Payload, &p) != nil || p.ObjectID == "" {
			return
		}
		if g, ok := gates(ctx); ok {
			if err := g.GateRefused(ctx, p); err != nil {
				slog.Warn("planification: a refused move's resolution was not planned", "object", p.ObjectID, "guard", p.Guard, "error", err)
			}
		}
		return
	case !strings.HasSuffix(ev.Topic, ".transitioned"):
		return
	}
	var p cwmapi.ObjectTransitionedEventPayload
	if json.Unmarshal(ev.Payload, &p) != nil {
		return
	}
	if p.Type == api.ReleaseType && p.To == api.ReleaseReleased {
		republish(ctx, p.ObjectID)
		return
	}
	if p.Capability == "" {
		if g, ok := gates(ctx); ok {
			if err := g.GateMoved(ctx, p.ObjectID, p.To, p.Transition); err != nil {
				slog.Warn("planification: a gate wait was not ended after its move", "object", p.ObjectID, "error", err)
			}
		}
		// A person's step that ends on this state (a decision resolved on
		// its own panel) is done, by whoever moved it.
		if err := plancore.Reached(ctx, p.ObjectID, p.To, p.By); err != nil {
			slog.Warn("planification: a step ending on its target's move was not ended", "object", p.ObjectID, "to", p.To, "error", err)
		}
		// A plan its refused end blocked ends once a person moved it done.
		if err := plancore.EndConfirmed(ctx, p.ObjectID, p.To); err != nil {
			slog.Warn("planification: a plan blocked by a refused end was not ended after its confirm", "object", p.ObjectID, "error", err)
		}
		// The last of what an increment holds moved past a request's end
		// (a person's confirm): its delivery is asked.
		if err := plancore.AskDeliveryOnMove(ctx, p.ObjectID); err != nil {
			slog.Warn("planification: an increment's delivery was not asked after a move", "object", p.ObjectID, "error", err)
		}
		// A goal or a feature dropped drops its work.
		if err := plancore.EndDroppedPlan(ctx, p.ObjectID, p.To); err != nil {
			slog.Warn("planification: a dropped want's plan was not ended", "object", p.ObjectID, "error", err)
		}
		// An increment dropped drops the work of what it took.
		if err := plancore.EndDroppedIncrement(ctx, p.ObjectID, p.To); err != nil {
			slog.Warn("planification: a dropped increment's work was not ended", "object", p.ObjectID, "error", err)
		}
		// A goal's follow refused while this feature was not done ends now.
		if err := plancore.EndFollowedGoals(ctx, p.ObjectID, p.To); err != nil {
			slog.Warn("planification: a followed goal was not ended after its feature's move", "object", p.ObjectID, "error", err)
		}
		// What depends on it may go now (a decision that applies).
		if err := plancore.SettleAll(ctx); err != nil {
			slog.Warn("planification: the work waiting on a moved object was not released", "object", p.ObjectID, "error", err)
		}
	}
	if p.Capability == api.CapPlan && p.To == api.PlanSplit {
		// Split past the requests service (a move by hand): its own steps go.
		if err := plancore.SettleAll(ctx); err != nil {
			slog.Warn("planification: the steps of a split request were not settled", "request", p.ObjectID, "error", err)
		}
	}
	if p.Capability == api.CapPlan && (p.To == api.PlanDone || p.To == api.PlanDropped) {
		// Ended past the plans service (a move by hand): its work goes too.
		if err := plancore.EndTodos(ctx, p.ObjectID, p.To, p.Note); err != nil {
			slog.Warn("planification: the to-dos of an ended request were not archived", "request", p.ObjectID, "error", err)
		}
		if cwm, err := application.UseService[cwmapi.CWMService](ctx); err == nil {
			if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: p.ObjectID}); err == nil {
				plancore.EndOrigin(ctx, got.Object, plancore.PlanningActor)
			}
		}
	}
	if p.Capability == api.CapPlan && (p.To == api.PlanDone || p.To == api.PlanDropped) {
		// What waited on it may go now (an end moved by hand, past the plans
		// service, included): every to-do is settled again.
		if err := plancore.SettleAll(ctx); err != nil {
			slog.Warn("planification: the work waiting on an ended request was not released", "request", p.ObjectID, "error", err)
		}
		// The module that asked for it awaits it no longer.
		if waits, err := application.UseService[schedapi.WaitService](ctx); err == nil {
			if _, err := waits.Clear(ctx, schedapi.ClearWaitInput{On: p.ObjectID}); err != nil {
				slog.Warn("planification: the wait on an ended request was not cleared", "request", p.ObjectID, "error", err)
			}
		}
		if g, ok := gates(ctx); ok {
			if err := g.GateRequestEnded(ctx, p.ObjectID); err != nil {
				slog.Warn("planification: the gate a request resolves was not tried again", "request", p.ObjectID, "error", err)
			}
		}
	}
}

// loopActivity is what Settle watches, by application: the observer's
// handlers in flight, and when the last one ended. One application's
// activity never holds another's settle (tests run their worlds in parallel).
// Keyed weakly: a closed application is not kept alive by its entry.
var loopActivity sync.Map // weak.Pointer[application.Application] -> *activity

type activity struct {
	inflight atomic.Int64
	last     atomic.Int64
	// handled is, by object id, the revision of the last write of it the
	// observer followed.
	handled sync.Map
}

// Handled says whether the observer of the application ctx carries has
// followed the write of object id that left revision: a caller that needs a
// reaction behind it (a test answering a question) waits on it, which Settle
// cannot do for an event still queued.
func Handled(ctx context.Context, id, revision string) bool {
	got, ok := activityOf(ctx).handled.Load(id)
	return ok && got == revision
}

// activityOf is the observer's activity of the application ctx carries.
func activityOf(ctx context.Context) *activity {
	key := weak.Make(application.AppFromContext(ctx))
	if a, ok := loopActivity.Load(key); ok {
		return a.(*activity)
	}
	a, _ := loopActivity.LoadOrStore(key, &activity{})
	return a.(*activity)
}

// Settle waits until the observer of the application ctx carries has been
// idle for a moment — no handler running, none ended in the last 50ms — or
// `max` has passed. The observer works on its own goroutine; a caller about
// to tear the workspace down (a test, a shutdown) settles it first so no
// adjustment writes behind it.
func Settle(ctx context.Context, max time.Duration) {
	act := activityOf(ctx)
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if act.inflight.Load() == 0 && time.Since(time.Unix(0, act.last.Load())) > 50*time.Millisecond {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TriggerGrace replaces every rule's grace by d, so a test need not wait a
// rule's minute; it answers the restore.
func TriggerGrace(d time.Duration) func() { return plancore.SetTriggerGrace(d) }

// answered wakes the requests that asked an inbox question just answered.
func answered(ctx context.Context, entry string) {
	svc, err := application.UseService[api.RequestService](ctx)
	if err != nil {
		return
	}
	w, ok := svc.(waker)
	if !ok {
		return
	}
	if woken, err := w.Answered(ctx, entry); err != nil {
		slog.Warn("planification: an answer did not wake its request", "question", entry, "woken", woken, "error", err)
	}
}

// fileRequestInput is what the chat agent fills: the request, without the
// actor — the tool files it as the chat's agent.
type fileRequestInput struct {
	Title        string            `json:"title"                  jsonschema:"The ask in one line"`
	Details      string            `json:"details,omitempty"      jsonschema:"What the one line leaves out"`
	Module       string            `json:"module,omitempty"       jsonschema:"The module it targets; leave empty when unsure (intake routes it)"`
	About        string            `json:"about,omitempty"        jsonschema:"The object it concerns (an id), when there is one"`
	Deliverables []api.Deliverable `json:"deliverables,omitempty" jsonschema:"What the work must produce: a title and how to check it"`
	BudgetUSD    float64           `json:"budget_usd,omitempty"   jsonschema:"Spend ceiling in US dollars"`
	DependsOn    []string          `json:"depends_on,omitempty"   jsonschema:"Ids of requests it waits on"`
}

// FileRequestTool files a request from a chat.
func FileRequestTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool("file-request",
		"File a request: work asked for in one line, with what it must deliver. It lands in the module it targets (or is routed); its intake is planned and evaluates it.",
		func(ctx context.Context, in fileRequestInput) (api.FileOutput, error) {
			svc, err := application.UseService[api.RequestService](ctx)
			if err != nil {
				return api.FileOutput{}, err
			}
			return svc.File(ctx, api.FileInput{
				Title: in.Title, Details: in.Details, Module: in.Module, About: in.About, Deliverables: in.Deliverables,
				BudgetUSD: in.BudgetUSD, DependsOn: in.DependsOn,
				By: cwmapi.Actor{ID: "chat", Kind: "agent"},
			})
		},
		toolsapi.WithMutating(),
	)
}

// AgentPluginExtension contributes the `file-request` chat tool and the
// `planification` plugin (the intake tools) — only in a binary that runs the
// agent loop (it owns the agent PluginFactory point).
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("planification-tools", "", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, toolsapi.ToolExtension(FileRequestTool()), planificationPluginStrategy)
		})
	}
}

// republish publishes again the publications a release makes out of date.
func republish(ctx context.Context, release string) {
	svc, err := application.UseService[api.PublicationService](ctx)
	if err != nil {
		slog.Warn("planification: publication service unavailable; nothing republished", "release", release, "error", err)
		return
	}
	r, ok := svc.(republisher)
	if !ok {
		return // a remote service: this process does not publish
	}
	if done, err := r.OnReleased(ctx, release); err != nil {
		slog.Warn("planification: republication failed", "release", release, "published", done, "error", err)
	}
}
