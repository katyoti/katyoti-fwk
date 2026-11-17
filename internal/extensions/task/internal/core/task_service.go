// SPDX-License-Identifier: Apache-2.0

// Package core provides the in-process TaskService implementation.
//
// The service is composed of single-concern units, each in its own file:
//   - run_registry.go   — the live/terminal run map (TTL+LRU), entry lifecycle
//   - service_tasks.go  — name/URI resolution (catalog, then ExtraDirs), List/Get facade
//   - exec_request.go   — TaskExecRequest building, factory + session-id resolve
//   - recorder_wiring.go— SessionRecorder ↔ EventBus subscription wiring
//   - metrics.go        — periodic queue-depth gauge emitter
//
// This file holds the thin Service struct, its construction, and the run
// lifecycle verbs (Run/Status/Stop/Result/Runs) that drive the executor.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/redact"
	"github.com/katyoti/katyoti-fwk/internal/core/services"
	agentlocal "github.com/katyoti/katyoti-fwk/internal/extensions/agent/local"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
	taskcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib"
)

// Service implements TaskService as thin composition over the concern
// units in this package.
type Service struct {
	// closing: Close has begun (shutdown.go) — no new run; servingProc: this
	// is the serving process's service (a clean stop pauses its runs).
	closing     atomic.Bool
	servingProc serving
	// startingMu: one runAs at a time — a child id is started once.
	startingMu sync.Mutex
	// bridge publishes the runs' transitions on the universe's bus; appCtx is
	// the application context it resolves the bus through.
	bridge lifecycleBridge
	appCtx context.Context
	// keepAlive: the tasks the universe keeps running (keepalive.go).
	keepAlive *keepAlive
	// panels: the panel server and subjects its runs' panels end through;
	// nil = the application's (run_panels.go; a test sets its own).
	panels *panelSeams
	// warnings: what went wrong around the task service (warnings.go).
	warnMu   sync.Mutex
	warnings []string
	// sessionsDir is where runs' session journals are written; empty = the
	// application's (tests set their own).
	sessionsDir string
	// tasks are the ExtraDirs' tasks (provider "user"): run when named after
	// the catalog (resolveTask), never listed (e24 / N1).
	tasks map[string]*taskapi.Task
	// library answers List/Get: the catalog only (projections.md, TaskLibrary
	// row), plus the skill-as-task wrappers.
	library taskapi.TaskLibrary
	channel *engine.Channel
	bgMgr   taskapi.BackgroundTaskManager
	suspend *suspensionManager // ask-user suspension inbox (user-comms concern)

	// groups limits how many runs of a task group run at once (TaskGroups.yaml;
	// groups.go). nil = no limit (tests building the service by hand).
	groups *groupScheduler

	// reg owns the run-state map + its TTL/LRU bounds + entry locking.
	reg *runRegistry

	// runs keeps each run's manifest and, while it waits, its checkpoint in
	// the DataDir (run_store.go): what a restart resumes or reports
	// interrupted. nil = nothing kept (tests building the service by hand).
	runs *runStore

	// mu guards the metric interval only (the run map has its own lock
	// inside reg). Periodic queue-depth metric emitter: engine.RunQueueDepthEvent
	// on engine.RunQueueDepthTopic. Started in NewTaskService; stopped when the
	// goroutine receives metricStop OR the application context cancels.
	mu             sync.RWMutex
	metricInterval time.Duration
	metricStop     chan struct{}
}

// SetRetentionTTL overrides the terminal-run retention window. Used by
// tests to drive fast pruning (e.g. 1ms) without waiting for the default.
func (s *Service) SetRetentionTTL(d time.Duration) { s.reg.setRetentionTTL(d) }

// SetLRUCap overrides the LRU count ceiling for the runs map. Tests use
// small caps (e.g. 3) to drive eviction without populating 1000+ entries.
func (s *Service) SetLRUCap(n int) { s.reg.setLRUCap(n) }

// QueueDepth returns the current run-entry count. Used by the periodic
// metric emitter.
func (s *Service) QueueDepth() int { return s.reg.len() }

// NewTaskService is the service factory.
//
// Two inputs, two questions (modules/contributions/projections.md):
//   - discovery (List, Get): the catalog this application publishes
//     (`mbapi.Catalog` — the self view in a binary without a universe, the
//     universe view where UniverseCatalogExtension is wired), plus the
//     skill-as-task wrappers read from that same catalog;
//   - execution (Run by name): the same catalog first, read through its lanes
//     (taskcontrib.CatalogTask), so an override entry it applies is the task
//     that runs; then `cfg.ExtraDirs` (ad-hoc filesystem roots, "user"
//     provider). A task found only there runs when named and is never
//     listed — ExtraDirs is execution-only, like SkillManager's.
func NewTaskService(ctx context.Context, cfg *TaskServiceConfig) (taskapi.TaskService, error) {
	var loaders []taskapi.TaskLoader
	for _, dir := range cfg.ExtraDirs {
		loaders = append(loaders, taskapi.TaskLoader{
			Loader:   loader.NewFSLoader(dir),
			Root:     ".",
			Provider: "user",
		})
	}
	tasks := taskcontrib.DiscoverTasksFrom(loaders)

	// NOTE: skill-as-task auto-wrapping (`provides: [task]`) is now handled
	// by skillTaskLibrary as a separate composite source. The previous
	// init-time addRunnableSkills call ran too early — the SkillManager
	// at AppInit only had local skills; remote sidecar skills are wrapped
	// in via sidecarDiscoveryExt at the later Bootstrap phase. Querying
	// the SkillManager lazily on each ListTasks call (skillTaskLibrary)
	// always sees the live composite. WrapTaskLibraryWithRemotes adds
	// it to the source chain.

	bus, _ := application.UseService[engine.EventBus](ctx)
	if bus == nil {
		bus = engine.NewMemoryEventBus()
	}
	channel := engine.NewChannel(bus)
	bgMgr := NewBackgroundTaskManager(channel)
	appCtx := services.ContextWithoutResolutionStack(ctx)

	svc := &Service{
		tasks:          tasks,
		library:        NewCompositeTaskLibrary(newAppCatalogTaskLibrary(appCtx), NewSkillTaskLibrary()),
		channel:        channel,
		bgMgr:          bgMgr,
		suspend:        newSuspensionManager(),
		reg:            newRunRegistry(),
		metricInterval: defaultMetricInterval,
		metricStop:     make(chan struct{}),
	}
	// TaskGroups.yaml; absent, every group runs DefaultMaxRunning at once.
	groupsCfg, gErr := application.UseConfig[taskapi.TaskGroupsConfig](ctx)
	svc.useGroups(newGroupScheduler(groupsCfg))
	if gErr != nil && !errors.Is(gErr, fs.ErrNotExist) {
		svc.serviceWarn("TaskGroups.yaml cannot be read — every group runs with the defaults: " + gErr.Error())
	}
	// The runs' manifests and waiting checkpoints: what a restart resumes or
	// reports interrupted (RestoreRuns, called when the process serves).
	if env := application.Env(); env != nil {
		if st, err := newRunStore(env.GetDataSubDir("runs")); err == nil {
			svc.useRuns(st)
		} else {
			svc.serviceWarn("the runs folder is unavailable — runs will not survive a restart: " + err.Error())
		}
	}
	svc.useKeepAlive(groupsCfg.KeepAlive) // after the runs store: its state is kept there
	// Wire circular reference: bgManager needs taskSvc + app context to run sub-tasks.
	// Clear the resolution stack: this factory ctx has TaskService(default) on it,
	// and background child runs use appCtx as their independent resolution root —
	// without clearing, any child resolving TaskService (e.g. the bubble_to_parent
	// ask-user channel reaching SuspensionManager) would falsely trip the
	// circular-dependency guard.
	bg := bgMgr.(*bgManager)
	bg.taskSvc = svc
	bg.appCtx = appCtx
	svc.appCtx = appCtx

	// Start the periodic queue-depth metric emitter. Stops when ctx cancels
	// or svc.Close() closes metricStop. See `decisions/run-state-lifecycle.md`.
	go svc.runMetricLoop(ctx)
	go svc.runWatchdog(ctx)

	return svc, nil
}

// Close stops the periodic metric goroutine. Safe to call multiple times.
// Application-scoped services usually rely on context cancellation, but
// tests and explicit shutdown paths benefit from a direct stop signal.
// BackgroundManager returns the BackgroundTaskManager for background agent lifecycle.
func (s *Service) BackgroundManager() taskapi.BackgroundTaskManager {
	return s.bgMgr
}

// SuspensionManager returns the ask-user suspension inbox (Suspend/Resume/
// Suspensions) — the user-comms wait surface, distinct from the background
// scheduler.
func (s *Service) SuspensionManager() taskapi.SuspensionManager {
	return s.suspend
}

// ── Run ──────────────────────────────────────────────────────────────────

func (s *Service) Run(ctx context.Context, in taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	if s.closing.Load() {
		return taskapi.TaskRunResponse{}, ErrShuttingDown
	}
	if in.Origin == nil || in.Origin.Kind != taskapi.OriginKeepAlive {
		s.keepAliveRunByHand(in.TaskName) // a stopped kept-alive task runs again
	}
	// Generate the run ID up front so the executor uses it AND the recorder can
	// filter events by it (each agent's recorder only writes events emitted by
	// its own run, preventing cross-agent leakage in nested delegations).
	// GenerateRunID() already returns the "run-{hex}" format used as emitter.
	return s.start(ctx, in, engine.GenerateRunID(), nil)
}

// start prepares and launches a new run under runID (site: where its parent
// started it, for a child).
func (s *Service) start(ctx context.Context, in taskapi.TaskRunRequest, runID string, site *spawnSite) (taskapi.TaskRunResponse, error) {
	p, err := s.prepare(ctx, in)
	if err != nil {
		return taskapi.TaskRunResponse{}, err
	}
	p.site = site
	entry, err := s.launch(p, runID, launchNew, time.Now())
	if err != nil {
		return taskapi.TaskRunResponse{}, err
	}
	// The run is live: its goroutine writes its entry — answer from a snapshot.
	// The full RunInfo-shaped response lets callers subscribe to bus events /
	// open journal panels without a follow-up Status round-trip; State is
	// omitted at start (callers query Status for live state).
	info, _, _ := s.reg.snapshot(entry.info.RunID)
	return responseOf(info), nil
}

// prepared is a run built and not started: its task, request and machine.
type prepared struct {
	ctx          context.Context // the caller's, with the effective permission profile
	in           taskapi.TaskRunRequest
	site         *spawnSite // where its parent started it (a child), saved in its manifest
	t            *taskapi.Task
	originalType string
	execRequest  *taskapi.TaskExecRequest
	machine      engine.Machine
}

// prepare resolves a request's task and creates its machine.
func (s *Service) prepare(ctx context.Context, in taskapi.TaskRunRequest) (*prepared, error) {
	t, err := s.resolveTask(ctx, in)
	if err != nil {
		return nil, err
	}

	if t.Type == "" {
		return nil, fmt.Errorf("task %q has no 'type' field", t.Name)
	}

	// Top-level dry-run substitution: when the request is dry-run AND the
	// task type is in the substitute set (agent, task, human-gate, program),
	// replace the type with simulator and the config with synthetic dry-run
	// output. Mirrors what LocalSpawner does for workflow children. Without
	// this, top-level `type: agent` tasks would still hit the LLM.
	originalType := t.Type
	if in.DryRunMode && DryRunSubstituted[t.Type] {
		t = &taskapi.Task{
			Name:   t.Name,
			Type:   "simulator",
			Config: DryRunSyntheticConfig(t.Name, originalType, in.Input),
			// Body intentionally cleared — simulator has no body.
		}
	}

	factory, err := s.getFactory(t.Type)
	if err != nil {
		return nil, err
	}

	// Effective permission profile, by precedence:
	//   1. an explicit profile already in ctx (CLI --permission-profile / GUI) — wins;
	//   2. the request-propagated profile (a delegating PARENT passes its active
	//      profile on the child request — children run on the bg app ctx, so the
	//      profile can't ride ctx into them);
	//   3. the task-declared default (TASK.md `permissionProfile:`).
	// The winner is injected onto ctx so it flows to the kernel gate via
	// runCtx → executor dispatch. This is how the compose wizard (and the
	// workers it delegates) author files outside the workspace subdir under a
	// broader `compose-edit` profile instead of being denied by `workspace-edit`.
	if agentlocal.PermissionProfileFrom(ctx) == "" {
		profile := in.PermissionProfile
		if profile == "" {
			profile = t.PermissionProfile
		}
		if profile != "" {
			ctx = agentlocal.WithPermissionProfile(ctx, profile)
		}
	}
	// The winner is also the request's, so the run's manifest keeps it: a run
	// restored after a restart runs under the profile it was started under.
	in.PermissionProfile = agentlocal.PermissionProfileFrom(ctx)

	// Resolve the session ID (caller > parent's session > fresh) BEFORE the
	// exec request is built, so factories see the final value in
	// Runtime.SessionID. The TaskRunResponse echoes it back so the caller
	// (TerminalService, ktw-gui, direct API users) knows exactly which
	// directory holds the journal.
	sessionID := s.resolveSessionID(in, t.Name)
	in.SessionID = sessionID

	// Build the separated request. The factory receives a *TaskExecRequest
	// with config, input, body, and runtime cleanly separated. If the factory's
	// strategy implements SchemaProvider, body is injected into config under the
	// declared field name.
	execRequest := buildExecRequest(t, in, factory)

	// The shared Channel, and the spawner a machine starts its children with:
	// every child a run of this service (child_runs.go).
	machineCtx := engine.WithSpawner(engine.WithChannel(ctx, s.channel), serviceSpawner{s})

	machine, err := factory.CreateMachine(machineCtx, execRequest)
	if err != nil {
		return nil, fmt.Errorf("create machine: %w", err)
	}

	return &prepared{ctx: ctx, in: in, t: t, originalType: originalType, execRequest: execRequest, machine: machine}, nil
}

// launchMode is how launch starts a run.
type launchMode int

const (
	launchNew            launchMode = iota // a new run, from its start
	launchResumeWaiting                    // restored after a restart, waiting again where it waited
	launchRestartWorking                   // working at a crash: started again from its last saved step
	launchRestartFresh                     // queued (or not past its first step) at a crash: started again
)

// launch starts a prepared run under runID, as how says.
func (s *Service) launch(p *prepared, runID string, how launchMode, startedAt time.Time) (*runEntry, error) {
	resuming := how == launchResumeWaiting
	ctx, in, t, execRequest, machine := p.ctx, p.in, p.t, p.execRequest, p.machine
	originalType, sessionID := p.originalType, p.in.SessionID

	// Its group: the caller's, else the task's, else the parent run's, else
	// the default (a person's). Admitted now so a full group refuses the run
	// at once. A resumed run waits: it holds no slot until its event comes.
	// What goes wrong before the run is registered: told once it is.
	var warnings []string
	group := ""
	if s.groups != nil {
		var groupWarning string
		group, groupWarning = s.groupFor(in, t)
		if groupWarning != "" {
			warnings = append(warnings, groupWarning)
		}
		if how == launchNew {
			// A child of a run already admitted queues, never refused: a full
			// queue must not fail a workflow half-way (its fan-out is bounded).
			admit := s.groups.admit
			if s.liveParent(in.ParentRunID) {
				admit = s.groups.admitChild
			}
			if err := admit(runID, group, in.Priority); err != nil {
				return nil, err
			}
		} else {
			s.groups.admitRestored(runID, group, in.Priority, resuming)
		}
	}

	// Wire session recorder for JSONL journals. Pass the resolved
	// sessionID so the on-disk directory matches what we return.
	recorder, recErr := s.openRecorder(ctx, t.Name, sessionID, runID)
	if recErr != nil {
		warnings = append(warnings, "its journal could not be opened — its History stays empty: "+recErr.Error())
	}

	// Framework-emitted task START — the symmetric bookend to the task.output
	// (DONE) event published below. Fired HERE, by the execution framework, so
	// every run reports its start with the full id set, resolved dimensions,
	// and input REGARDLESS of how it was launched (direct, delegate, workflow).
	// A caller can never skip it (the bug that hid agent_as_human children).
	if s.channel != nil && how == launchNew {
		mode := configString(execRequest.Config, "mode")
		started := &engine.TaskStartedEvent{
			RunID:      runID,
			TaskName:   t.Name,
			TaskType:   originalType,
			Mode:       mode,
			Dimensions: dimensionsForMode(mode),
			Input:      in.Input,
		}
		started.Correlation = engine.Correlation{SessionID: sessionID, RunID: runID, ParentRunID: in.ParentRunID}
		started.StampEvent(engine.EventTaskStarted, runID)
		s.channel.Bus().Publish(engine.EventTaskStarted, started)
	}
	if p.site != nil && how == launchNew {
		s.tellChildStarted(runID, in, p.site, t.Type)
	}

	var store engine.StateStore
	if s.runs != nil {
		store = s.runs
		if engine.RestartsAfterCrash(machine) {
			s.runs.keepEveryStep(runID)
		}
		if how == launchNew {
			m := runManifest{RunID: runID, Request: in, Group: group, StartedAt: startedAt, Owner: s.runs.owner, PID: os.Getpid(), TaskHash: taskHash(t), Spawn: p.site, Queued: true}
			if err := s.runs.saveManifest(m); err != nil {
				warnings = append(warnings, "its manifest could not be saved — a restart will not see it: "+err.Error())
			}
		}
	}
	if store != nil {
		store = tellingStore{StateStore: store, s: s}
	}
	// Its context: cancelled by Stop, or failed with a cause by what the run
	// needs and cannot have (an event it cannot be told) — then its error.
	// It keeps the caller's values but not its cancellation: the call that
	// started it ends when it has answered the run id (a KTSP call's context
	// is cancelled then), the run lives on. A child of a live parent of this
	// process keeps its parent's context: it ends with its parent.
	base := ctx
	if !s.liveParent(in.ParentRunID) {
		base = context.WithoutCancel(ctx)
	}
	runCtx, failRun := context.WithCancelCause(engine.WithRunWarner(base, func(reason string) { s.warn(runID, reason) }))
	cancel := func() { failRun(nil) }
	// Its maximum duration is a deadline from when it was submitted — waiting
	// and queueing count — kept across a restart (a restored run keeps its
	// start).
	if d := deadlineOf(in, startedAt); !d.IsZero() {
		var stopDeadline context.CancelFunc
		runCtx, stopDeadline = context.WithDeadline(runCtx, d)
		cancel = func() { stopDeadline(); failRun(nil) }
	}
	executor := newRunExecutor(machine, s.channel, store, runID, s.groups, failRun)
	executor.RegisterHook(lifecycleHook{s: s, runID: runID})
	// A restored run is reported restored once it listens again: from then on
	// an event published for it is heard (its wait is subscribed).
	var listening chan struct{}
	if resuming {
		listening = make(chan struct{})
		executor.RegisterHook(&onFirstSuspend{fn: func() { close(listening) }})
	}

	entry := &runEntry{
		info: taskapi.RunInfo{
			RunID:       runID,
			TaskName:    t.Name,
			MachineType: t.Type,
			Status:      initialStatus(resuming, s.groups != nil),
			ParentRunID: in.ParentRunID,
			SessionID:   sessionID,
			StartedAt:   startedAt,
			Group:       group,
			Priority:    in.Priority,
			Origin:      originOf(in),
			Target:      in.Target,
		},
		request: in,
		cancel:  cancel,
		pause:   func() { failRun(ErrPausedByShutdown) },
		done:    make(chan struct{}),
		killed:  make(chan struct{}),
	}

	s.reg.add(entry)
	for _, w := range warnings {
		s.warn(runID, w)
	}
	// Whoever cannot call Stop (a terminal, another process) asks for it on
	// the run's cancel-request topic.
	stopAsked := s.channel.Bus().Subscribe(engine.CancelRequestTopic(runID), func(string, any) {
		go func() { _, _ = s.Stop(context.Background(), taskapi.StopRequest{RunID: runID}) }() // not on the bus's goroutine: Stop publishes
	})

	go func() {
		// Last, once it has ended: what was kept for it while it ran goes —
		// its answered question, the notifications of its children (one
		// arriving after is not queued: its parent is gone).
		defer s.releaseEnded(runID)
		defer close(entry.done)
		defer stopAsked()
		defer cancel()
		if s.runs != nil { // a run paused by a clean stop stays on disk for the next start
			defer func() {
				if !errors.Is(context.Cause(runCtx), ErrPausedByShutdown) {
					s.runs.forget(runID)
				}
			}()
		}
		if s.groups != nil {
			defer s.groups.done(runID)
		}
		// Closed however the run ends — also when it is stopped while queued.
		if recorder != nil {
			defer recorder.Close()
		}
		// A new run starts when its group has a slot for it; a resumed one
		// takes its slot when its event comes (the slot hook's OnResume).
		// One that never gets it — stopped, out of time, paused by a clean
		// stop while queued — ends through the same path as any run: its end
		// kept and told (a parent waiting on it hears it), or paused.
		var notStarted error
		if s.groups != nil && !resuming {
			notStarted = s.groups.acquire(runCtx, runID)
		}
		if notStarted == nil && !resuming && s.runs != nil {
			if err := s.runs.markBegan(runID); err != nil {
				s.warn(runID, "its start not written: "+err.Error())
			}
		}
		switch {
		case notStarted != nil:
		case how == launchResumeWaiting:
			s.tell(runID, engine.RunResumed, nil)
		case how == launchRestartWorking:
			s.tell(runID, engine.RunRestarted, func(e *engine.RunLifecycleEvent) { e.Reason = "from its last step" })
		case how == launchRestartFresh:
			s.tell(runID, engine.RunRestarted, func(e *engine.RunLifecycleEvent) { e.Reason = "from its beginning" })
		}

		// File tools (file-read/edit/write) resolve a SESSION-scoped filestate
		// Store to enforce read-before-write. The top-level run gets a session
		// scope from ServeTask, but a DELEGATED child runs on the background app
		// context with NO session scope — so its file-edit/file-write fail with
		// "filestate cache not registered". Enter one here when absent so every
		// run (parent or delegated worker) has its own Store.
		runScopeCtx := runCtx
		if services.SessionScope.ContainerFromContext(runCtx) == nil {
			scopedCtx, sessContainer := services.SessionScope.EnterScope(runCtx)
			runScopeCtx = scopedCtx
			defer sessContainer.Close()
		}

		// Ignored on purpose: entering a transient scope cannot fail, and the
		// function returns nil — the run's outcome is written inside it.
		_ = services.TaskScope.RunInScope(runScopeCtx, func(taskCtx context.Context) error {
			// A panic in the run (its machine, a tool, the model client) fails
			// this run with the panic — never the process, and every run in it.
			result, err := runRecovered(runID, func() (*engine.RunResult, error) {
				if notStarted != nil {
					return nil, notStarted
				}
				switch how {
				case launchResumeWaiting:
					return executor.ResumeWaiting(taskCtx, runID)
				case launchRestartWorking:
					return executor.ResumeWorking(taskCtx, runID)
				default:
					return executor.Run(taskCtx, engine.Event{Type: "start", Payload: execRequest})
				}
			})
			// A step that failed ends the run with its error in the result: it is
			// the run's error — its status, Result, task.output and lifecycle say why.
			if err == nil && result != nil && result.Next == engine.Failed && result.Error != nil {
				err = result.Error
			}
			// A machine that ends with a Failure (a workflow whose step failed)
			// has failed — its output, kept, says what it had done.
			if err == nil && result != nil && result.Failure != nil {
				err = errors.New(result.Failure.Reason)
			}
			if errors.Is(err, context.DeadlineExceeded) && runCtx.Err() == context.DeadlineExceeded {
				err = fmt.Errorf("%w: it ran out of time (max_duration_s %d)", ErrDeadlineExceeded, in.MaxDurationS)
			}
			// Paused by a clean stop: not an end — kept on disk, taken up at the
			// next start (shutdown.go).
			if errors.Is(context.Cause(runCtx), ErrPausedByShutdown) {
				s.reg.mutate(runID, func(e *runEntry) { e.terminalAt = time.Now() })
				s.tell(runID, engine.RunPaused, func(e *engine.RunLifecycleEvent) { e.Reason = ErrPausedByShutdown.Error() })
				return nil
			}
			// Failed with a cause (not stopped): the cause is its error.
			if errors.Is(err, context.Canceled) {
				if cause := context.Cause(runCtx); cause != nil && !errors.Is(cause, context.Canceled) {
					err = cause
				}
			}
			if notStarted != nil && errors.Is(err, context.Canceled) {
				err = errStoppedWhileQueued
				var q *queuedError
				if errors.As(notStarted, &q) { // why it never had a slot
					err = fmt.Errorf("%w: %s", errStoppedWhileQueued, q.Why)
				}
			}

			var finalStatus taskapi.RunStatus
			var finalTaskName string
			killedBefore := false
			s.reg.mutate(runID, func(e *runEntry) {
				e.result = result
				e.err = err
				killedBefore = e.info.Status == taskapi.RunStatusKilled
				switch {
				case e.stopped: // stopped by a person: cancelled now it has ended (killed stays)
					if e.info.Status == taskapi.RunStatusStopping || notStarted != nil {
						e.info.Status = taskapi.RunStatusCancelled
					}
				case errors.Is(err, errStoppedWhileQueued): // its parent's end, while queued
					e.info.Status = taskapi.RunStatusCancelled
				case err != nil:
					e.info.Status = taskapi.RunStatusFailed
				case result != nil && result.Next == engine.Done:
					e.info.Status = taskapi.RunStatusCompleted
				default:
					e.info.Status = taskapi.RunStatusFailed
				}
				if e.terminalAt.IsZero() {
					e.terminalAt = time.Now()
				}
				finalStatus = e.info.Status
				finalTaskName = e.info.TaskName
			})

			if killedBefore { // its end was told when it was killed: not twice
				return nil
			}
			if p.site != nil { // a child: its parent's observers see it end
				s.tellChildEnded(runID, in, p.site, finalStatus, result, err)
			}
			// Its result is kept before its end is told: whoever is woken by
			// it reads it, after a restart too.
			if s.runs != nil {
				res := savedResult{ParentRunID: in.ParentRunID, EndedAt: time.Now(), Request: &in}
				res.RunID, res.Status = runID, string(finalStatus)
				res.Duration = time.Since(entry.info.StartedAt).Round(time.Millisecond).String()
				if result != nil {
					res.Output = result.Output
				}
				if err != nil {
					res.Error = err.Error()
				}
				s.runs.saveResult(res)
			}
			s.tell(runID, endTransition(finalStatus), func(e *engine.RunLifecycleEvent) {
				if err != nil {
					e.Reason = err.Error()
				}
			})

			// Publish a terminal task.output event so post-hoc consumers (L2 test
			// runner, ktw-utils session show) can read the Output map from the
			// journal. The recorder subscription filters by emitter == runID so
			// each run's event lands only in its own session journal.
			if s.channel != nil {
				var outputMap map[string]any
				if result != nil {
					if m, ok := result.Output.(map[string]any); ok {
						outputMap = m
					}
				}
				errMsg := ""
				if err != nil {
					errMsg = err.Error()
				}
				evt := &engine.TaskOutputEvent{
					RunID:      runID,
					TaskName:   finalTaskName,
					Status:     string(finalStatus),
					Output:     outputMap,
					Error:      errMsg,
					DurationMs: time.Since(entry.info.StartedAt).Milliseconds(),
				}
				evt.Correlation = engine.Correlation{SessionID: sessionID, RunID: runID, ParentRunID: in.ParentRunID}
				evt.StampEvent(engine.EventTaskOutput, runID)
				s.channel.Bus().Publish(engine.EventTaskOutput, evt)
				// And on the run's own topic: whoever waits on this run as a
				// machine (a delegating parent) is woken.
				s.channel.Bus().Publish(engine.RunEndedTopic(runID), evt)
			}

			// If this run spawned in-process background children (background delegates or teammates),
			// wait for them before we let the parent run fully end. Without this, a CLI run can exit
			// immediately after the parent completes, killing in-process children before their agent
			// loop even starts.
			// A run that did not complete — failed, stopped — stops them:
			// orphans would go on spending (an agent's tokens) for nothing.
			if bg, ok := s.bgMgr.(*bgManager); ok && entry.info.RunID != "" {
				if finalStatus != taskapi.RunStatusCompleted {
					bg.killChildren(entry.info.RunID)
				}
				// No run holds a slot while it waits on another: its children
				// may need one in its group (limit 1 would never end).
				if s.groups != nil {
					s.groups.done(runID)
				}
				bg.waitChildren(taskCtx, entry.info.RunID)
			}
			return nil
		})
	}()
	if listening != nil {
		select {
		case <-listening:
		case <-entry.done:
		case <-time.After(restoreListenTimeout):
			slog.Warn("tasks: restore: a restored run is not listening yet", "run_id", runID)
		}
	}
	return entry, nil
}

// ErrRunPanicked is the error of a run whose code panicked.
var ErrRunPanicked = errors.New("the run panicked")

// runRecovered runs fn; a panic becomes ErrRunPanicked with its value (the
// stack is logged).
func runRecovered(runID string, fn func() (*engine.RunResult, error)) (result *engine.RunResult, err error) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("tasks: a run panicked", "run_id", runID, "panic", p, "stack", string(debug.Stack()))
			result, err = nil, fmt.Errorf("%w: %v", ErrRunPanicked, p)
		}
	}()
	return fn()
}

// restoreListenTimeout bounds how long a restore waits for a restored run to
// subscribe its wait again.
const restoreListenTimeout = 5 * time.Second

// openRecorder opens a run's session journal and wires it to the bus: what
// the run (and its subtree) does is written there. Nil when it cannot be.
func (s *Service) openRecorder(ctx context.Context, taskName, sessionID, runID string) (*engine.SessionRecorder, error) {
	dir := s.sessionsDir
	if dir == "" {
		dir = application.Env().SessionsDir()
	}
	recorder, err := engine.NewSessionRecorder(dir, taskName, sessionID)
	if recorder == nil {
		return nil, err
	}
	// Secrets must not land verbatim in journals (threat-T10 redaction
	// half). Settings come from Redaction.yaml; the zero value means
	// enabled with the built-in catalog. NOTE: the engine event plane is
	// in-process today — if session events are ever bridged to NATS,
	// that bridge must route through the same redactor BEFORE publish.
	settings, _ := application.UseConfig[redact.RedactionSettings](ctx)
	recorder.SetRedactor(redact.New(settings))
	subscribeRecorder(s.channel.Bus(), recorder, runID)
	return recorder, nil
}

// newRunExecutor builds the executor of every run of this package — a run of
// the TaskService or a workflow's child (LocalSpawner) — with the hooks a run
// needs wherever it runs: its group slot given back while it waits (groups
// non-nil) and the external events it waits on (an inbox answer) bridged onto
// the bus. One builder, so a machine that waits as a machine is woken the same
// way everywhere.
func newRunExecutor(machine engine.Machine, channel *engine.Channel, store engine.StateStore, runID string, groups *groupScheduler, fail func(error)) *engine.Executor {
	executor := engine.NewExecutor(machine, channel, store)
	executor.SetRunID(runID)
	executor.SetPanelPublisher(publishPanels)
	if groups != nil {
		executor.RegisterHook(slotHook{groups: groups, runID: runID})
	}
	if channel != nil {
		executor.RegisterHook(newEventBridgeHook(channel.Bus(), fail))
	}
	return executor
}

// onFirstSuspend calls fn once, the first time the run waits.
type onFirstSuspend struct {
	once sync.Once
	fn   func()
}

func (*onFirstSuspend) OnPreStep(context.Context, *engine.State, engine.Event) error         { return nil }
func (*onFirstSuspend) OnPostStep(context.Context, *engine.State, *engine.StepResult, error) {}
func (*onFirstSuspend) OnResume(context.Context, string, engine.Event)                       {}
func (h *onFirstSuspend) OnSuspend(context.Context, string, *engine.WaitSpec)                { h.once.Do(h.fn) }

// initialStatus: a restored run waits; a new run is queued until its group
// gives it a slot (running at once when no groups decide).
func initialStatus(resuming, grouped bool) taskapi.RunStatus {
	switch {
	case resuming:
		return taskapi.RunStatusWaiting
	case grouped:
		return taskapi.RunStatusQueued
	default:
		return taskapi.RunStatusRunning
	}
}

// ── Status / Stop / Result / Runs ────────────────────────────────────────

func (s *Service) Status(_ context.Context, in taskapi.StatusRequest) (taskapi.StatusResponse, error) {
	info, state, ok := s.reg.snapshot(in.RunID)
	if !ok {
		// Ended and gone from the registry (a restart, its retention): how
		// it ended, from its kept result.
		if s.runs != nil {
			if res, err := s.runs.loadResult(in.RunID); err == nil {
				return taskapi.StatusResponse{RunID: in.RunID, Status: taskapi.RunStatus(res.Status), ParentRunID: res.ParentRunID}, nil
			}
		}
		return taskapi.StatusResponse{}, fmt.Errorf("run %q not found", in.RunID)
	}
	return taskapi.StatusResponse{
		RunID:       info.RunID,
		TaskName:    info.TaskName,
		MachineType: info.MachineType,
		Status:      info.Status,
		ParentRunID: info.ParentRunID,
		SessionID:   info.SessionID,
		StartedAt:   info.StartedAt,
		Duration:    time.Since(info.StartedAt).Round(time.Millisecond).String(),
		State:       state,
	}, nil
}

func (s *Service) Stop(_ context.Context, in taskapi.StopRequest) (taskapi.StopResponse, error) {
	entry, ok := s.reg.get(in.RunID)
	if !ok {
		return taskapi.StopResponse{}, fmt.Errorf("run %q not found", in.RunID)
	}
	select {
	case <-entry.done:
		return taskapi.StopResponse{Stopped: false}, nil // already ended
	default:
	}
	// Cancel: the run stops at its next context check (a wait ends, an LLM
	// call returns); a queued run leaves its queue at once.
	entry.cancel()
	s.reg.mutate(in.RunID, func(e *runEntry) {
		e.info.Status = taskapi.RunStatusStopping // cancelled once it has ended
		e.stopped = true
	})
	s.tell(in.RunID, engine.RunStopping, nil)
	// On disk at once: a restart before it has ended reports it cancelled —
	// a stopped run never comes back.
	if s.runs != nil {
		if err := s.runs.markStopped(in.RunID, "stopped by a person"); err != nil {
			s.warn(in.RunID, "its stop could not be recorded — a restart before it ends may take it up again: "+err.Error())
		}
	}
	if info, _, ok := s.reg.snapshot(in.RunID); ok {
		s.keepAliveStopped(info)
	}
	// Its children are stopped with it (and marked so: a restart does not
	// revive them).
	s.stopChildren(in.RunID)
	// Kill: still alive after the grace period, it is let go — its slot
	// freed, its result ignored (a goroutine cannot be stopped from outside).
	killAfter := KillAfter
	go func(id string, done <-chan struct{}) {
		select {
		case <-done:
		case <-time.After(killAfter):
			s.reg.mutate(id, func(e *runEntry) {
				e.info.Status = taskapi.RunStatusKilled
				e.killedWhy = killedReason(killAfter)
				e.terminalAt = time.Now()
				e.killedOnce.Do(func() { close(e.killed) })
			})
			if s.groups != nil {
				s.groups.done(id)
			}
			if bg, ok := s.bgMgr.(*bgManager); ok {
				bg.killChildren(id)
			}
			s.tell(id, engine.RunKilled, func(e *engine.RunLifecycleEvent) { e.Reason = killedReason(killAfter) })
			// Its end, once, now — its goroutine may never return: kept, and
			// told to whoever waits on it (a parent).
			s.endKilled(id, killedReason(killAfter))
		}
	}(in.RunID, entry.done)
	return taskapi.StopResponse{Stopped: true}, nil
}

func (s *Service) Result(ctx context.Context, in taskapi.ResultRequest) (taskapi.ResultResponse, error) {
	entry, ok := s.reg.get(in.RunID)
	if !ok {
		// Ended and gone from the registry (a restart, its retention): its
		// kept result.
		if s.runs != nil {
			if res, err := s.runs.loadResult(in.RunID); err == nil {
				return res.ResultResponse, nil
			}
		}
		return taskapi.ResultResponse{}, fmt.Errorf("run %q not found", in.RunID)
	}

	// A run awaiting another's result waits: it gives its group's slot back
	// meanwhile (a parent delegating holds nothing while its child runs — no
	// deadlock when both are in one full group) and takes it again after.
	caller := engine.RunIDFromContext(ctx)
	waiting := false
	if s.groups != nil && caller != "" && caller != in.RunID {
		select {
		case <-entry.done:
		default:
			s.groups.release(caller)
			waiting = true
		}
	}
	// A bounded wait (WaitS) answers the run's status now when it passes.
	var bound <-chan time.Time
	if in.WaitS > 0 {
		timer := time.NewTimer(time.Duration(in.WaitS) * time.Second)
		defer timer.Stop()
		bound = timer.C
	}
	killed, notEnded := false, false
	select {
	case <-entry.done:
	case <-entry.killed:
		killed = true
	case <-bound:
		notEnded = true
	case <-ctx.Done():
		return taskapi.ResultResponse{}, ctx.Err()
	}
	if waiting {
		if err := s.groups.acquire(ctx, caller); err != nil {
			return taskapi.ResultResponse{}, err
		}
	}

	resp := taskapi.ResultResponse{
		RunID:    in.RunID,
		Duration: time.Since(entry.info.StartedAt).Round(time.Millisecond).String(),
	}
	if notEnded { // not ended within WaitS: where it is now, and why it still queues
		if info, _, ok := s.reg.snapshot(in.RunID); ok {
			resp.Status, resp.QueuedBecause = string(info.Status), info.QueuedBecause
		}
		if resp.Status == string(taskapi.RunStatusQueued) && s.groups != nil {
			if why := s.groups.queuedReason(in.RunID); why != "" { // as it stands now (a group paused live since)
				resp.QueuedBecause = why
			}
		}
		return resp, nil
	}
	if killed {
		// Its goroutine may still run (and write its entry): answer from the kill.
		resp.Status = string(taskapi.RunStatusKilled)
		if info, _, ok := s.reg.snapshot(in.RunID); ok {
			resp.Error = info.Error // why, as written when it was killed
		}
		return resp, nil
	}

	// Ended: its status as everyone else reads it (a stopped run is
	// cancelled, not failed), its error, and its output when it has one (a
	// failed workflow's says what it had done).
	resp.Status = string(entry.info.Status)
	if entry.err != nil {
		resp.Error = entry.err.Error()
	}
	if entry.result != nil {
		resp.Output = entry.result.Output
	}
	return resp, nil
}

// configString extracts a string field from a task config (a map[string]any
// after decode). Returns "" when absent or not a string.
func configString(cfg any, key string) string {
	if m, ok := cfg.(map[string]any); ok {
		if v, ok := m[key].(string); ok {
			return v
		}
	}
	return ""
}

// dimensionsForMode resolves the named delegation mode to its 6-dimension
// SpawnConfig and marshals it to a generic map for the task.started event
// (keeps the engine event package free of a presets import). Empty/unknown
// mode falls back to the worker preset.
func dimensionsForMode(mode string) map[string]any {
	cfg, err := presets.ResolvePreset(presets.Mode(mode), nil)
	if err != nil {
		return nil
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

func (s *Service) Runs(_ context.Context, in taskapi.ListRunsRequest) (taskapi.ListRunsResponse, error) {
	all := s.reg.list(in.SessionID)
	// Runs that ended before this process started are listed too.
	if s.runs != nil {
		live := make(map[string]bool, len(all))
		for _, r := range all {
			live[r.RunID] = true
		}
		for _, r := range s.runs.endedRuns() {
			if !live[r.RunID] && (in.SessionID == "" || r.SessionID == in.SessionID) {
				all = append(all, r)
			}
		}
	}
	out := make([]taskapi.RunInfo, 0, len(all))
	for _, r := range all {
		if in.Group != "" && r.Group != in.Group {
			continue
		}
		switch in.Status {
		case "":
		case "active":
			if r.Status != taskapi.RunStatusQueued && r.Status != taskapi.RunStatusRunning && r.Status != taskapi.RunStatusWaiting && r.Status != taskapi.RunStatusStopping {
				continue
			}
		default:
			if string(r.Status) != in.Status {
				continue
			}
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return taskapi.ListRunsResponse{Runs: out}, nil
}

// useGroups installs the group scheduler: the run registry's status follows
// its slots, and the suspension manager's in-tool waits give slots back.
func (s *Service) useGroups(g *groupScheduler) {
	s.groups = g
	g.onState = func(runID string, st taskapi.RunStatus, why string) {
		var started bool
		var queuedFor time.Duration
		s.reg.mutate(runID, func(e *runEntry) {
			if e.terminalAt.IsZero() && !e.stopped {
				e.info.Status = st
				e.info.QueuedBecause = why
			}
			switch {
			case st == taskapi.RunStatusQueued && e.queuedAt.IsZero():
				e.queuedAt = time.Now()
			case st == taskapi.RunStatusRunning && !e.started:
				e.started, started = true, true
				e.lastStepAt = time.Now() // it works from now (it may have queued long)
				if !e.queuedAt.IsZero() {
					queuedFor = time.Since(e.queuedAt)
				}
			}
		})
		switch {
		case st == taskapi.RunStatusQueued:
			s.tell(runID, engine.RunQueued, func(e *engine.RunLifecycleEvent) { e.Reason = why })
		case started:
			s.tell(runID, engine.RunStarted, func(e *engine.RunLifecycleEvent) { e.QueuedForMs = queuedFor.Milliseconds() })
		}
	}
	if s.suspend != nil {
		s.suspend.slots = g
		s.suspend.tell = func(runID string, t engine.RunTransition, what string) {
			s.tell(runID, t, func(e *engine.RunLifecycleEvent) {
				if t == engine.RunWaiting {
					e.Waiting = []string{what}
				} else {
					e.WokenBy = what
				}
			})
		}
	}
}

// groupFor is a new run's group: the request's, else the task's declared
// group, else its parent run's, else the default group.
// A group named but not declared is a warning: the run goes in the default.
func (s *Service) groupFor(in taskapi.TaskRunRequest, t *taskapi.Task) (group, warning string) {
	named := s.namedGroup(in, t)
	group = s.groups.resolve(named)
	if named != "" && group != named {
		warning = fmt.Sprintf("it names group %q, which TaskGroups.yaml does not declare — it runs in %q", named, group)
	}
	return group, warning
}

// namedGroup is the group a run names: the request's, else the task's, else
// its parent run's; "" when none.
func (s *Service) namedGroup(in taskapi.TaskRunRequest, t *taskapi.Task) string {
	if in.Group != "" {
		return in.Group
	}
	if t != nil && t.Group != "" {
		return t.Group
	}
	if in.ParentRunID != "" {
		return s.groups.groupOf(in.ParentRunID)
	}
	return ""
}

// slotHook gives a run's group slot back when its machine waits through the
// executor, and takes it again (queueing) before the machine resumes.
type slotHook struct {
	groups *groupScheduler
	runID  string
}

func (slotHook) OnPreStep(context.Context, *engine.State, engine.Event) error         { return nil }
func (slotHook) OnPostStep(context.Context, *engine.State, *engine.StepResult, error) {}
func (h slotHook) OnSuspend(context.Context, string, *engine.WaitSpec)                { h.groups.release(h.runID) }
func (h slotHook) OnResume(ctx context.Context, _ string, _ engine.Event) {
	_ = h.groups.acquire(ctx, h.runID) // a cancelled ctx fails the next step
}

// KillAfter is how long a stopped run may take to end before it is let go.
var KillAfter = 10 * time.Second

// originOf is who starts a run: the request's origin, else the parent run (an
// agent delegating), else a person.
func originOf(in taskapi.TaskRunRequest) taskapi.RunOrigin {
	switch {
	case in.Origin != nil && in.Origin.Kind != "":
		return *in.Origin
	case in.ParentRunID != "":
		return taskapi.RunOrigin{Kind: taskapi.OriginAgent, ID: in.ParentRunID}
	default:
		return taskapi.RunOrigin{Kind: taskapi.OriginPerson}
	}
}

// Groups is every task group's load and limits.
func (s *Service) Groups(context.Context, taskapi.GroupsRequest) (taskapi.GroupsResponse, error) {
	if s.groups == nil {
		return taskapi.GroupsResponse{Groups: []taskapi.GroupLoad{}, Warnings: s.serviceWarnings(), KeepAlive: s.keepAliveInfo()}, nil
	}
	return taskapi.GroupsResponse{Groups: s.groups.loads(), Warnings: s.serviceWarnings(), KeepAlive: s.keepAliveInfo()}, nil
}

// SetGroup changes a group's limits live, or pauses / resumes it.
func (s *Service) SetGroup(_ context.Context, in taskapi.SetGroupRequest) (taskapi.GroupLoad, error) {
	if in.Group == "" {
		return taskapi.GroupLoad{}, fmt.Errorf("SetGroup: group required")
	}
	if s.groups == nil {
		return taskapi.GroupLoad{}, fmt.Errorf("SetGroup: no task groups in this process")
	}
	if s.groups.resolve(in.Group) != in.Group {
		return taskapi.GroupLoad{}, fmt.Errorf("SetGroup: group %q is not declared in TaskGroups.yaml", in.Group)
	}
	return s.groups.set(in), nil
}

// ClearGroup removes the queued runs of a group — all, or one origin's — by
// stopping them (a queued run leaves its queue at once).
func (s *Service) ClearGroup(ctx context.Context, in taskapi.ClearGroupRequest) (taskapi.ClearGroupResponse, error) {
	out := taskapi.ClearGroupResponse{Cleared: []string{}}
	if in.Group == "" {
		return out, fmt.Errorf("ClearGroup: group required")
	}
	if s.groups == nil {
		return out, nil
	}
	// The runs the registry shows queued in the group — also one submitted an
	// instant ago that has not reached its group's queue yet (it is queued from
	// the moment Run returns).
	for _, r := range s.reg.list("") {
		if r.Group != in.Group || r.Status != taskapi.RunStatusQueued || (in.OriginID != "" && r.Origin.ID != in.OriginID) {
			continue
		}
		if _, err := s.Stop(ctx, taskapi.StopRequest{RunID: r.RunID}); err == nil {
			out.Cleared = append(out.Cleared, r.RunID)
		}
	}
	return out, nil
}

// endKilled keeps a killed run's result and tells its end on its run's topic —
// once, when it is killed (its goroutine, let go, may never return).
func (s *Service) endKilled(runID, why string) {
	info, _, ok := s.reg.snapshot(runID)
	if !ok {
		return
	}
	if s.runs != nil {
		var req taskapi.TaskRunRequest
		s.reg.mutate(runID, func(e *runEntry) { req = e.request })
		res := savedResult{ParentRunID: info.ParentRunID, EndedAt: time.Now(), Request: &req}
		res.RunID, res.Status, res.Error = runID, string(taskapi.RunStatusKilled), why
		s.runs.saveResult(res)
	}
	if s.channel != nil {
		evt := &engine.TaskOutputEvent{RunID: runID, TaskName: info.TaskName, Status: string(taskapi.RunStatusKilled), Error: why}
		evt.Correlation = engine.Correlation{SessionID: info.SessionID, RunID: runID, ParentRunID: info.ParentRunID}
		evt.StampEvent(engine.EventTaskOutput, runID)
		s.channel.Bus().Publish(engine.EventTaskOutput, evt)
		s.channel.Bus().Publish(engine.RunEndedTopic(runID), evt)
	}
}

// errStoppedWhileQueued is the end of a run stopped before it had a slot.
var errStoppedWhileQueued = errors.New("stopped while queued")
