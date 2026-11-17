// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/oslock"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE STORE — a module's to-dos are files in its planning space: `.ktw/todo/`
// while they are scheduled, `.ktw/done/` once collected, and its settings in
// `.ktw/settings.yaml`. Every write holds the module's lock (an operating-system
// lock on a file, see locked) and lands through a rename, so two processes —
// the universe and a command line, two sessions — never write over each other.

const (
	todoDir      = "todo"
	doneDir      = "done"
	waitingDir   = "waiting"
	settingsFile = "settings.yaml"
	lockFile     = ".lock"
)

// cwmOf resolves the CWM service.
func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return nil, fmt.Errorf("the CWM service: %w", err)
	}
	return cwm, nil
}

// planningSpace is a module's `.ktw` folder: beside its objects folder.
func planningSpace(ctx context.Context, module string) (string, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return "", err
	}
	got, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: module})
	if projectless(module, err) {
		return universeSpace(ctx)
	}
	if err != nil {
		return "", fmt.Errorf("module %s: %w", module, err)
	}
	caps, err := effectiveCapabilities(ctx, cwm, module, got.ID)
	if err != nil {
		return "", fmt.Errorf("module %s: %w", module, err)
	}
	if err := plannable(module, got.Type, caps.Capabilities); err != nil {
		return "", err
	}
	dir := filepath.Dir(got.Path)
	if filepath.Base(dir) == "objects" {
		dir = filepath.Dir(dir)
	}
	return filepath.Join(dir, ".ktw"), nil
}

// effectiveCapabilities reads a resolved module's capabilities in the project
// its reference names: a qualified reference asks by its qualified id, which
// the store set routes to that project - a bare id there is
// refused once several projects hold it. A single project's store knows its
// objects by their bare id only: a qualified id it does not hold is asked
// again bare.
func effectiveCapabilities(ctx context.Context, cwm cwmapi.CWMService, ref, id string) (cwmapi.ObjectEffectiveCapabilitiesOutput, error) {
	key, _, err := cwmapi.SplitReference(ref)
	if err != nil || key == "" {
		return cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: id})
	}
	caps, err := cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: cwmapi.QualifiedID(key, id)})
	if errors.Is(err, cwmapi.ErrNotFound) {
		return cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: id})
	}
	return caps, err
}

// projectless says the requests module is served by no project: the CWM
// knows no such module - a universe with no attached project, the ordinary
// self-contained layout (cenv.AttachedProjects), or a project without it. Its
// to-dos (a delegated child's question to its parent) are then
// the universe's, kept beside its runs (universeSpace).
func projectless(module string, err error) bool {
	return module == api.RequestsModule && errors.Is(err, cwmapi.ErrNotFound)
}

// universeSpace is the planning space of the requests module when no project
// serves it: a folder of the universe's scheduler folder (SchedulerDir), never
// one derived from a checkout.
func universeSpace(ctx context.Context) (string, error) {
	env, err := application.UseService[application.AppEnvironment](ctx)
	if err != nil || env == nil || env.SchedulerDir() == "" {
		return "", fmt.Errorf("%w: module %s: no project serves it and the environment names no scheduler folder (SchedulerDir)", cwmapi.ErrNotFound, api.RequestsModule)
	}
	return filepath.Join(env.SchedulerDir(), api.RequestsModule), nil
}

// plannable refuses a planning space to what is not a module (it carries no
// module capability) and to a system module: a system module holds the
// platform's operational state, never to-dos.
func plannable(module, typ string, caps []cwmapi.CapabilityRef) error {
	var isModule, isSystem bool
	for _, c := range caps {
		isModule = isModule || c.ID == cwmapi.CapModule
		isSystem = isSystem || c.ID == cwmapi.CapSystemModule
	}
	switch {
	case !isModule:
		return fmt.Errorf("%w: %s is a %s, not a module", cwmapi.ErrInvalidInput, module, typ)
	case isSystem && bareModule(module) != api.RequestsModule:
		return fmt.Errorf("%w: %s is a system module: it holds no to-dos", cwmapi.ErrInvalidInput, module)
	}
	return nil
}

// modules is the reference of every module whose to-dos the scheduler keeps,
// in order: every user module, and the requests module of each project that
// has one - or, when no project serves it, the universe's once it holds any
// (the other system modules hold no to-dos). A reference is named as the CWM
// names it: bare while one project is served, qualified
// <key>:<module> when several are (ObjectListOutput.Projects) - two projects
// each holding a module of one name are two references, never one
func modules(ctx context.Context) ([]string, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return nil, err
	}
	all, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: cwmapi.CapModule, Limit: -1})
	if err != nil {
		return nil, err
	}
	sys, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: cwmapi.CapSystemModule, Limit: -1})
	if err != nil {
		return nil, err
	}
	system := map[string]bool{}
	for i := range sys.Objects {
		system[listedRef(sys, i)] = true
	}
	var out []string
	requests := false
	for i := range all.Objects {
		ref := listedRef(all, i)
		if system[ref] && all.Objects[i].ID != api.RequestsModule {
			continue
		}
		requests = requests || all.Objects[i].ID == api.RequestsModule
		out = append(out, ref)
	}
	if !requests {
		// No project serves it: the universe's own, once it holds any.
		if space, err := universeSpace(ctx); err == nil {
			if _, err := os.Stat(space); err == nil {
				out = append(out, api.RequestsModule)
			}
		}
	}
	sort.Strings(out)
	return slices.Compact(out), nil
}

// listedRef is the reference of a listed object: qualified by the project
// holding it when the list names each object's project (several served).
func listedRef(l cwmapi.ObjectListOutput, i int) string {
	if len(l.Projects) == len(l.Objects) && l.Projects[i] != "" {
		return cwmapi.QualifiedID(l.Projects[i], l.Objects[i].ID)
	}
	return l.Objects[i].ID
}

// lockWait is how long a writer waits for a module's lock before it gives up.
var lockWait = 10 * time.Second

// locked runs fn holding a module's lock: an operating-system lock on
// `.ktw/todo/.lock` (core/oslock), held for as long as its holder lives. A
// live holder keeps it however long it works - its age takes nothing from it -
// and a dead one's is free at once, released by the system. No process removes
// the file: it stays, its note naming its last holder.
func locked(space string, fn func() error) error {
	dir := filepath.Join(space, todoDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, lockFile)
	deadline := time.Now().Add(lockWait)
	for {
		l, err := oslock.TryLock(path)
		if err == nil {
			_ = l.Note(fmt.Sprintf("pid %d", os.Getpid()))
			defer l.Unlock()
			return fn()
		}
		if !errors.Is(err, oslock.ErrLocked) {
			return err
		}
		if time.Now().After(deadline) {
			who := oslock.Note(path)
			if who == "" {
				who = "another writer"
			}
			return fmt.Errorf("the to-dos of %s are locked by %s (%s)", space, who, path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// newID is a to-do's id: its module, then a token ordered by time (to the
// microsecond: the order they were made in).
func newID(module string) string { return idOf(module, "TD") }

// idOf is an id of the store: its module, its kind, then a token ordered by
// time.
func idOf(module, kind string) string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	t := time.Now().UTC()
	return module + "." + kind + "-" + t.Format("20060102-150405") + fmt.Sprintf("%06d", t.Nanosecond()/1000) + "-" + hex.EncodeToString(b)
}

// moduleOfID is the module a to-do id names.
func moduleOfID(id string) (string, error) {
	m, _, ok := strings.Cut(id, ".TD-")
	if !ok || m == "" {
		return "", fmt.Errorf("%w: %q is not a to-do id (<module>.TD-…)", cwmapi.ErrInvalidInput, id)
	}
	return m, nil
}

// fileOf is a to-do's file name.
func fileOf(id string) string {
	_, local, _ := strings.Cut(id, ".")
	return local + ".yaml"
}

// read reads one to-do file.
func read(path string) (api.Todo, error) { return readAs[api.Todo](path) }

// write writes a to-do file through a rename.
func write(dir string, t api.Todo) error { return writeAs(dir, t.ID, t) }

// todoCache keeps each to-do file's last reading with its size and time: a
// listing reads and parses only the files changed since the last one, the
// rest answered from memory (- a planning view lists every
// module's to-dos several times).
var todoCache = struct {
	sync.Mutex
	dirs map[string]map[string]cachedTodo
}{dirs: map[string]map[string]cachedTodo{}}

type cachedTodo struct {
	mod  time.Time
	size int64
	todo api.Todo
}

// listDir reads every to-do of a folder, by id - an unchanged file from the
// cache, each answered as its own copy.
func listDir(dir string) ([]api.Todo, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	todoCache.Lock()
	seen := todoCache.dirs[dir]
	todoCache.Unlock()
	fresh := make(map[string]cachedTodo, len(entries))
	var out []api.Todo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		info, infoErr := e.Info()
		if infoErr == nil {
			if c, ok := seen[e.Name()]; ok && c.size == info.Size() && c.mod.Equal(info.ModTime()) {
				fresh[e.Name()] = c
				out = append(out, cloneTodo(c.todo))
				continue
			}
		}
		path := filepath.Join(dir, e.Name())
		t, err := read(path)
		if errors.Is(err, os.ErrNotExist) {
			// Moved away meanwhile (archived, removed) - or being replaced by
			// another process this very instant: read once more before skipping.
			time.Sleep(10 * time.Millisecond)
			if t, err = read(path); errors.Is(err, os.ErrNotExist) {
				continue
			}
		}
		if err != nil {
			return nil, err
		}
		if infoErr == nil {
			fresh[e.Name()] = cachedTodo{mod: info.ModTime(), size: info.Size(), todo: cloneTodo(t)}
		}
		out = append(out, t)
	}
	todoCache.Lock()
	todoCache.dirs[dir] = fresh
	todoCache.Unlock()
	return out, nil
}

// cloneTodo is a to-do no other reader shares: its slices, its work and its
// claim and result copied.
func cloneTodo(t api.Todo) api.Todo {
	t.After, t.Reads, t.Writes, t.Labels = slices.Clone(t.After), slices.Clone(t.Reads), slices.Clone(t.Writes), slices.Clone(t.Labels)
	t.Work = maps.Clone(t.Work)
	if t.Claim != nil {
		c := *t.Claim
		t.Claim = &c
	}
	if t.Result != nil {
		r := *t.Result
		r.More = maps.Clone(r.More)
		t.Result = &r
	}
	return t
}

// readAs reads one file of the store.
func readAs[T any](path string) (T, error) {
	var v T
	raw, err := atomicio.ReadAtomic(path) // shared: never in a writer's way
	if err != nil {
		return v, err
	}
	// A to-do is data, not configuration: no ${…} expansion in its texts.
	if err := marshal.Unmarshal(raw, marshal.YAML, &v); err != nil {
		return v, fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}

// writeAs writes one file of the store through a rename that replaces it even
// while a reader holds it open.
func writeAs(dir, id string, v any) error {
	raw, err := marshal.Marshal(v, marshal.YAML)
	if err != nil {
		return err
	}
	return atomicio.WriteAtomic(filepath.Join(dir, fileOf(id)), raw, 0o644)
}

// listAs reads every file of a folder of the store, by name (by id).
func listAs[T any](dir string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []T
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		v, err := readAs[T](path)
		if errors.Is(err, os.ErrNotExist) {
			// Moved away meanwhile (archived, removed) — or being replaced by
			// another process this very instant: read once more before skipping.
			time.Sleep(10 * time.Millisecond)
			if v, err = readAs[T](path); errors.Is(err, os.ErrNotExist) {
				continue
			}
		}
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// settings reads a module's settings: manual, done kept 30 days, waits of 7
// days, unless it says otherwise.
func settings(space string) (api.Settings, error) {
	s := api.Settings{Scheduling: api.ModeManual, DoneDays: api.DefaultDoneDays, WaitDays: api.DefaultWaitDays}
	raw, err := atomicio.ReadAtomic(filepath.Join(space, settingsFile))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	var own api.Settings
	if err := config.DecodeBytes(raw, marshal.YAML, &own); err != nil {
		return s, fmt.Errorf("%s: %w", filepath.Join(space, settingsFile), err)
	}
	if own.Scheduling != "" {
		s.Scheduling = own.Scheduling
	}
	if own.DoneDays > 0 {
		s.DoneDays = own.DoneDays
	}
	if own.WaitDays > 0 {
		s.WaitDays = own.WaitDays
	}
	return s, nil
}

// writeSettings writes a module's settings.
func writeSettings(space string, s api.Settings) error {
	if err := os.MkdirAll(space, 0o755); err != nil {
		return err
	}
	raw, err := marshal.Marshal(s, marshal.YAML)
	if err != nil {
		return err
	}
	return atomicio.WriteAtomic(filepath.Join(space, settingsFile), raw, 0o644)
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
