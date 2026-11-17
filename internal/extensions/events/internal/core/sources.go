// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// Sources raises the events that are not object writes: a new
// git HEAD (code.changed on the modules whose code packages hold its paths),
// a recurring timer come due (timer.elapsed), a delayed event at its time, a
// call from outside. Every raise goes through CWM's EventRaise with its cause;
// the cursors (the last HEAD seen, each timer's last firing) are this
// extension's own uncommitted state, never CWM's.
type Sources struct {
	mu sync.Mutex
	// stateDir is where the cursors live: <data folder>/events by default.
	stateDir string
	// repo overrides the git repository read (default: the project root).
	repo string
	now  func() time.Time
}

// SourcesOptions tunes the sources (tests point them at a temp folder and a
// clock).
type SourcesOptions struct {
	StateDir string
	Repo     string
	Now      func() time.Time
}

// NewSources builds the sources.
func NewSources(o SourcesOptions) *Sources {
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Sources{stateDir: o.StateDir, repo: o.Repo, now: o.Now}
}

var _ api.EventSourceService = (*Sources)(nil)

// cursors is the sources' state between polls.
type cursors struct {
	Repo   string            `json:"repo,omitempty"`
	Head   string            `json:"head,omitempty"`
	Timers map[string]string `json:"timers,omitempty"` // <object>#<timer> → last firing (RFC 3339)
}

func (s *Sources) statePath() string {
	dir := s.stateDir
	if dir == "" {
		dir = filepath.Join(application.Env().DataDir(), "events")
	}
	return filepath.Join(dir, "sources.json")
}

func (s *Sources) load() cursors {
	var c cursors
	if b, err := os.ReadFile(s.statePath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if c.Timers == nil {
		c.Timers = map[string]string{}
	}
	return c
}

func (s *Sources) save(c cursors) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.statePath()), 0o755); err != nil {
		return err
	}
	return atomicio.WriteAtomic(s.statePath(), b, 0o644)
}

// Poll steps every source once: the code, the recurring timers, the delayed
// events due.
func (s *Sources) Poll(ctx context.Context, _ api.PollInput) (api.PollOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.PollOutput{}, err
	}
	c := s.load()
	out := api.PollOutput{Raised: []api.Raised{}}
	if err := s.pollCode(ctx, cwm, &c, &out); err != nil {
		slog.Warn("events: the code source", "error", err)
	}
	if err := s.pollTimers(ctx, cwm, &c, &out); err != nil {
		slog.Warn("events: the timers", "error", err)
	}
	if err := s.save(c); err != nil {
		return out, err
	}
	if err := s.runDelayed(ctx, cwm, &out); err != nil {
		slog.Warn("events: the delayed events", "error", err)
	}
	return out, nil
}

// --- the code source ---------------------------------------------------------

func (s *Sources) pollCode(ctx context.Context, cwm cwmapi.CWMService, c *cursors, out *api.PollOutput) error {
	repo := s.repo
	if repo == "" {
		repo = projectRoot(ctx, cwm)
	}
	if repo == "" {
		return nil
	}
	head, err := git(repo, "rev-parse", "HEAD")
	if err != nil {
		return nil // not a repository, or no commit yet: no code event
	}
	out.Head = head
	if c.Repo != repo || c.Head == "" {
		c.Repo, c.Head = repo, head // first sight: the cursor is set, nothing raised
		return nil
	}
	if head == c.Head {
		return nil
	}
	from := c.Head
	c.Head = head
	changed, err := git(repo, "diff", "--name-only", from, head)
	if err != nil {
		return fmt.Errorf("diff %s..%s: %w", short(from), short(head), err)
	}
	var commits []api.Commit
	if log, err := git(repo, "log", "--format=%H%x09%s", from+".."+head); err == nil {
		for _, l := range lines(log) {
			sha, subject, _ := strings.Cut(l, "\t")
			commits = append(commits, api.Commit{SHA: sha, Subject: subject})
		}
	}
	paths := lines(changed)
	mods, err := codeModules(ctx, cwm)
	if err != nil {
		return err
	}
	rctx := cause.WithCause(ctx, cause.Cause{Kind: api.CauseCode, ID: head})
	for _, m := range mods {
		mine := pathsOf(paths, m.packages)
		if len(mine) == 0 {
			continue
		}
		payload, _ := json.Marshal(api.CodeChanged{Repo: repo, From: from, To: head, Commits: commits, Paths: mine})
		ev, err := cwm.EventRaise(rctx, cwmapi.EventRaiseInput{Type: api.EventCodeChanged, ObjectID: m.id, Payload: payload})
		if err != nil {
			return fmt.Errorf("raise code.changed on %s: %w", m.id, err)
		}
		out.Raised = append(out.Raised, api.Raised{Type: api.EventCodeChanged, Object: m.id, EventID: ev.EventID, Detail: strings.Join(mine, ", ")})
	}
	return nil
}

type codeModule struct {
	id       string
	packages []string
}

// codeModules is every module object with code packages.
func codeModules(ctx context.Context, cwm cwmapi.CWMService) ([]codeModule, error) {
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: "code"}) // a module with code
	if err != nil {
		return nil, err
	}
	var out []codeModule
	for _, o := range list.Objects {
		got, err := getObject(ctx, cwm, o.ID)
		if err != nil {
			continue
		}
		f, _, _ := cwmapi.DecodeCapability[struct {
			CodePackages []string `json:"code_packages"`
		}](got.Object, "code")
		if len(f.CodePackages) > 0 {
			out = append(out, codeModule{id: o.ID, packages: f.CodePackages})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out, nil
}

// pathsOf is the paths a module's code packages hold: a package is a folder
// (a prefix) or a file.
func pathsOf(paths, packages []string) []string {
	var out []string
	for _, p := range paths {
		for _, pkg := range packages {
			pkg = strings.TrimPrefix(filepath.ToSlash(pkg), "./")
			dir := strings.TrimSuffix(pkg, "/")
			if p == dir || strings.HasPrefix(p, dir+"/") {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// projectRoot is the attached project's folder, as CWM resolves its objects:
// any module (an object carrying the module capability) resolves to it.
func projectRoot(ctx context.Context, cwm cwmapi.CWMService) string {
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: cwmapi.CapModule})
	if err != nil || len(list.Objects) == 0 {
		return ""
	}
	r, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: list.Objects[0].ID})
	if err != nil {
		return ""
	}
	return r.ProjectRoot
}

// git answers a git command's output, trimmed.
func git(dir string, args ...string) (string, error) {
	out, err := gitcli.Run(dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func lines(s string) []string { return gitcli.Lines(s) }

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// --- the timers ---------------------------------------------------------------

func (s *Sources) pollTimers(ctx context.Context, cwm cwmapi.CWMService, c *cursors, out *api.PollOutput) error {
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: api.CapTimed})
	if err != nil {
		return err
	}
	now := s.now()
	for _, o := range list.Objects {
		got, err := getObject(ctx, cwm, o.ID)
		if err != nil {
			continue
		}
		f, _, err := cwmapi.DecodeCapability[api.TimedFields](got.Object, api.CapTimed)
		if err != nil {
			continue
		}
		for _, tm := range f.Timers {
			key := o.ID + "#" + tm.Name
			last, seen := c.Timers[key]
			due, ok := dueAt(tm, last, seen, now)
			if !seen {
				c.Timers[key] = now.UTC().Format(time.RFC3339Nano) // first sight: counted from now
				if !ok {
					continue
				}
			}
			if !ok {
				continue
			}
			payload, _ := json.Marshal(api.TimerElapsed{Timer: tm.Name, Due: due.UTC().Format(time.RFC3339)})
			rctx := cause.WithCause(ctx, cause.Cause{Kind: api.CauseTimer, ID: key})
			ev, err := cwm.EventRaise(rctx, cwmapi.EventRaiseInput{Type: api.EventTimerElapsed, ObjectID: o.ID, Payload: payload})
			if err != nil {
				return fmt.Errorf("raise timer.elapsed on %s: %w", o.ID, err)
			}
			c.Timers[key] = now.UTC().Format(time.RFC3339Nano)
			out.Raised = append(out.Raised, api.Raised{Type: api.EventTimerElapsed, Object: o.ID, EventID: ev.EventID, Detail: tm.Name})
		}
	}
	return nil
}

// dueAt says whether a timer is due now, and when it was due. Every: one
// period after its last firing (never on first sight). At: once a day at that
// time of day — due when today's time has passed and it has not fired since
// (on first sight too, once that time has passed: a firing missed while the
// universe was down fires once).
func dueAt(tm api.Timer, last string, seen bool, now time.Time) (time.Time, bool) {
	lastAt, _ := time.Parse(time.RFC3339Nano, last)
	switch {
	case tm.Every != "":
		d, err := time.ParseDuration(tm.Every)
		if err != nil || d <= 0 || !seen {
			return time.Time{}, false
		}
		due := lastAt.Add(d)
		return due, !now.Before(due)
	case tm.At != "":
		hm, err := time.ParseInLocation("15:04", tm.At, now.Location())
		if err != nil {
			return time.Time{}, false
		}
		today := time.Date(now.Year(), now.Month(), now.Day(), hm.Hour(), hm.Minute(), 0, 0, now.Location())
		if now.Before(today) {
			return time.Time{}, false
		}
		if seen && !lastAt.Before(today) {
			return time.Time{}, false
		}
		return today, true
	}
	return time.Time{}, false
}

// --- delayed events ---------------------------------------------------------

// After puts a delayed event as a keyed to-do of the object's module, ready at
// its time; the poll (or the internal worker, in an auto module) raises it then.
func (s *Sources) After(ctx context.Context, in api.AfterInput) (api.AfterOutput, error) {
	if in.Type == "" || in.Object == "" || in.After == "" {
		return api.AfterOutput{}, fmt.Errorf("%w: event-sources: After needs a type, an object and a delay", cwmapi.ErrInvalidInput)
	}
	d, err := time.ParseDuration(in.After)
	if err != nil || d <= 0 {
		return api.AfterOutput{}, fmt.Errorf("%w: event-sources: after %q is not a positive duration", cwmapi.ErrInvalidInput, in.After)
	}
	module, err := cwmapi.ModuleOfObjectID(in.Object)
	if err != nil {
		return api.AfterOutput{}, err
	}
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return api.AfterOutput{}, err
	}
	due := s.now().Add(d).UTC()
	key := in.Key
	if key == "" {
		key = "after:" + in.Type + "@" + in.Object + "@" + due.Format(time.RFC3339Nano)
	}
	got, err := todos.Put(ctx, schedapi.PutInput{Todo: schedapi.Todo{
		Module: module, Target: in.Object, Title: "delayed " + in.Type + " on " + in.Object,
		For: schedapi.ForAgent, Group: api.ReactionGroup, Start: schedapi.StartAuto, Priority: cwmapi.DefaultAsyncPriority,
		State: schedapi.StateReady, Reads: []string{module}, Owner: api.Owner, Key: key,
		NotBefore: due.Format(time.RFC3339Nano),
		Work:      map[string]any{api.WorkDoer: api.WorkDoerDelayed, "type": in.Type, "object": in.Object, "payload": string(in.Payload)},
	}})
	if err != nil {
		return api.AfterOutput{}, err
	}
	return api.AfterOutput{Todo: got.Todo.ID, Due: got.Todo.NotBefore}, nil
}

// runDelayed raises the delayed events that are due: each pulled, raised and
// reported, so it runs once whoever else may look.
func (s *Sources) runDelayed(ctx context.Context, cwm cwmapi.CWMService, out *api.PollOutput) error {
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return err
	}
	all, err := todos.List(ctx, schedapi.ListInput{State: schedapi.StateReady})
	if err != nil {
		return err
	}
	now := s.now()
	for _, t := range all.Todos {
		if t.Owner != api.Owner || t.Work[api.WorkDoer] != api.WorkDoerDelayed {
			continue
		}
		if at, err := time.Parse(time.RFC3339Nano, t.NotBefore); err == nil && now.Before(at) {
			continue
		}
		p, err := todos.Pull(ctx, schedapi.PullInput{ID: t.ID, By: sourcesActor})
		if err != nil || p.Todo == nil {
			continue
		}
		ev, err := raiseDelayed(ctx, cwm, *p.Todo, sourcesActor)
		if err != nil {
			continue
		}
		out.Raised = append(out.Raised, api.Raised{Type: fmt.Sprint(t.Work["type"]), Object: t.Target, EventID: ev, Detail: "delayed"})
	}
	return nil
}

// sourcesActor is who the sources act as on the scheduler.
var sourcesActor = cwmapi.Actor{ID: "event-sources", Kind: "agent"}

// raiseDelayed raises a delayed event's to-do (held by `by`) and reports it.
func raiseDelayed(ctx context.Context, cwm cwmapi.CWMService, t schedapi.Todo, by cwmapi.Actor) (string, error) {
	typ, _ := t.Work["type"].(string)
	obj, _ := t.Work["object"].(string)
	payload, _ := t.Work["payload"].(string)
	var raw json.RawMessage
	if payload != "" {
		raw = json.RawMessage(payload)
	}
	rctx := cause.WithCause(ctx, cause.Cause{Kind: api.CauseTimer, ID: t.Key})
	ev, err := cwm.EventRaise(rctx, cwmapi.EventRaiseInput{Type: typ, ObjectID: obj, Payload: raw})
	status, summary := schedapi.StatusSuccess, "raised "+typ+" on "+obj
	if err != nil {
		status, summary = schedapi.StatusFail, "raising "+typ+" on "+obj+" failed: "+err.Error()
	}
	if rerr := report(ctx, t.ID, status, summary, by); rerr != nil && err == nil {
		err = rerr
	}
	return ev.EventID, err
}

// --- from outside ---------------------------------------------------------

// Raise raises a declared event from outside (through the hub): cause external.
func (s *Sources) Raise(ctx context.Context, in api.RaiseInput) (api.Raised, error) {
	if in.Type == "" || in.Object == "" || in.By.ID == "" {
		return api.Raised{}, fmt.Errorf("%w: event-sources: Raise needs a type, an object and who raises it", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.Raised{}, err
	}
	rctx := cause.WithCause(ctx, cause.Cause{Kind: api.CauseExternal, ID: in.By.ID})
	ev, err := cwm.EventRaise(rctx, cwmapi.EventRaiseInput{Type: in.Type, ObjectID: in.Object, Payload: in.Payload})
	if err != nil {
		return api.Raised{}, err
	}
	return api.Raised{Type: in.Type, Object: in.Object, EventID: ev.EventID, Detail: "by " + in.By.ID}, nil
}

// getObject reads one object by id (its module is its id's first segment).
func getObject(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.ObjectGetOutput, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.ObjectGetOutput{}, err
	}
	return cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
}
