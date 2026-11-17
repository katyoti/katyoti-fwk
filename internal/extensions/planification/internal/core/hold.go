// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/oslock"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// ON HOLD — « Mettre en attente »: the person's flag on one item of their
// queue, keyed by the item's key. A held item leaves its action and the count
// and is listed last, under « En attente »; « Reprendre » takes it back. The
// flag is the queue's alone: it lives apart, in sys-planification's planning
// space (`.ktw/holds.yaml`), never on what the item is about — that keeps its
// state, what waits on it stays waiting, no agent takes it as answered. A
// hold whose item is gone is dropped at the next write. A universe serving
// several projects keeps a holds file in each project's sys-planification:
// an item's hold goes to the project holding it (PersonItem.Project), the
// first served project's when it names none; every file is read
//

const (
	holdsFile     = "holds.yaml"
	holdsLockFile = ".holds.lock"
)

// holdsDoc is the holds file.
type holdsDoc struct {
	Holds []api.Hold `json:"holds"`
}

// Hold puts one item of the person's queue on hold, by a person.
func (s *Service) Hold(ctx context.Context, in api.HoldInput) (api.HoldOutput, error) {
	relayed, err := personActs(ctx, "Hold", in.Key, in.By)
	if err != nil {
		return api.HoldOutput{}, err
	}
	items, err := personQueue(ctx)
	if err != nil {
		return api.HoldOutput{}, err
	}
	i := slices.IndexFunc(items, func(it api.PersonItem) bool { return it.Key == in.Key })
	if i < 0 {
		return api.HoldOutput{}, invalid("Hold: no item %q waits on the person", in.Key)
	}
	h := api.Hold{Key: in.Key, Object: items[i].Object, Reason: strings.TrimSpace(in.Reason), By: in.By,
		At: time.Now().UTC().Format(time.RFC3339), Relayed: relayed}
	live := map[string]bool{}
	for _, it := range items {
		live[it.Key] = true
	}
	err = editHolds(ctx, items[i].Project, func(hs []api.Hold) []api.Hold {
		hs = slices.DeleteFunc(hs, func(x api.Hold) bool {
			return x.Key == in.Key || (!live[x.Key] && !strings.HasPrefix(x.Key, incrementHoldKey))
		})
		return append(hs, h)
	})
	if err != nil {
		return api.HoldOutput{}, err
	}
	return api.HoldOutput{Hold: &h}, nil
}

// Unhold takes an item on hold back to its action, by a person.
func (s *Service) Unhold(ctx context.Context, in api.UnholdInput) (api.HoldOutput, error) {
	if _, err := personActs(ctx, "Unhold", in.Key, in.By); err != nil {
		return api.HoldOutput{}, err
	}
	found, err := dropHolds(ctx, in.Key)
	if err != nil {
		return api.HoldOutput{}, err
	}
	if !found {
		return api.HoldOutput{}, invalid("Unhold: %q is not on hold", in.Key)
	}
	return api.HoldOutput{}, nil
}

// personActs refuses what is not a person's own act: an actor of another
// kind, or a call from an agent's session whatever by says - unless it relays
// that person's act, confirmed (personsAct); the relay is answered to record.
func personActs(ctx context.Context, verb, key string, by cwmapi.Actor) (*api.Relayed, error) {
	if err := checkActor(by); err != nil {
		return nil, err
	}
	if by.Kind != "person" {
		return nil, invalid("%s: the person's queue is the person's - not %s %s", verb, by.Kind, by.ID)
	}
	return personsAct(ctx, by, strings.ToLower(verb)+":"+key, verb+": a person puts their items on hold themselves (« À faire », or their own command)")
}

// holdsOf reads the holds by key, of every served project's file; none when
// there is no file, nor a sys-planification to hold it.
func holdsOf(ctx context.Context) (map[string]api.Hold, error) {
	out := map[string]api.Hold{}
	for _, key := range holdProjects(ctx) {
		path, err := holdsPath(ctx, key)
		if errors.Is(err, cwmapi.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		hs, err := readHolds(path)
		if err != nil {
			return nil, err
		}
		for _, h := range hs {
			out[h.Key] = h
		}
	}
	return out, nil
}

// holdProjects is the projects whose sys-planification holds a holds file:
// the served projects' keys, the first served first, when several are
// served; one "" (the served project) when one is - read once for a view
// carrying the read cache.
func holdProjects(ctx context.Context) []string {
	c, cached := ctx.Value(readCacheKey{}).(*readCache)
	if cached {
		c.mu.Lock()
		keys, read := c.holdKeys, c.holdKeysRead
		c.mu.Unlock()
		if read {
			return keys
		}
	}
	keys := []string{""}
	if cwm, err := cwmOf(ctx); err == nil {
		if got, err := cwm.ProjectsKnown(ctx, cwmapi.ProjectsKnownInput{}); err == nil {
			var served []string
			for _, p := range got.Projects {
				if p.Served && p.ID != "" {
					served = append(served, p.ID)
				}
			}
			if len(served) > 1 {
				keys = served
			}
		}
	}
	if cached {
		c.mu.Lock()
		c.holdKeys, c.holdKeysRead = keys, true
		c.mu.Unlock()
	}
	return keys
}

// editHolds rewrites the holds file of the project of key (the first served
// project's when key is empty) under its lock, its sys-planification made
// first when the project has none.
func editHolds(ctx context.Context, key string, edit func([]api.Hold) []api.Hold) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	if key == "" {
		key = holdProjects(ctx)[0]
	}
	if _, err := cwmapi.EnsureSystemModule(ctx, cwm, systemModuleOf(key), systemModuleDescription); err != nil {
		return err
	}
	path, err := holdsPath(ctx, key)
	if err != nil {
		return err
	}
	return rewriteHolds(path, edit)
}

// dropHolds removes the hold of key from every holds file holding it, and
// says whether one did.
func dropHolds(ctx context.Context, key string) (bool, error) {
	found := false
	for _, project := range holdProjects(ctx) {
		path, err := holdsPath(ctx, project)
		if errors.Is(err, cwmapi.ErrNotFound) {
			continue
		}
		if err != nil {
			return found, err
		}
		if err := rewriteHolds(path, func(hs []api.Hold) []api.Hold {
			return slices.DeleteFunc(hs, func(x api.Hold) bool {
				found = found || x.Key == key
				return x.Key == key
			})
		}); err != nil {
			return found, err
		}
	}
	return found, nil
}

// systemModuleOf is sys-planification in the project of key: bare when key
// is empty.
func systemModuleOf(key string) string {
	if key == "" {
		return api.SystemModule
	}
	return cwmapi.QualifiedID(key, api.SystemModule)
}

// rewriteHolds rewrites the holds file at path under its lock.
func rewriteHolds(path string, edit func([]api.Hold) []api.Hold) error {
	return holdsLocked(filepath.Dir(path), func() error {
		hs, err := readHolds(path)
		if err != nil {
			return err
		}
		raw, err := marshal.Marshal(holdsDoc{Holds: edit(hs)}, marshal.YAML)
		if err != nil {
			return err
		}
		return atomicio.WriteAtomic(path, raw, 0o644)
	})
}

func readHolds(path string) ([]api.Hold, error) {
	raw, err := atomicio.ReadAtomic(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc holdsDoc
	if err := config.DecodeBytes(raw, marshal.YAML, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc.Holds, nil
}

// holdsPath is the holds file of the project of key (the served one when
// empty): in its sys-planification's planning space, beside its objects
// folder.
func holdsPath(ctx context.Context, key string) (string, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return "", err
	}
	module := systemModuleOf(key)
	got, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: module})
	if err != nil {
		return "", fmt.Errorf("the holds of the person's queue live in %s: %w", module, err)
	}
	dir := filepath.Dir(got.Path)
	if filepath.Base(dir) == "objects" {
		dir = filepath.Dir(dir)
	}
	return filepath.Join(dir, ".ktw", holdsFile), nil
}

// holdsLockWait is how long a writer of the holds waits for their lock before
// it gives up.
var holdsLockWait = 10 * time.Second

// holdsLocked runs fn holding the holds' lock, so a command line and the
// universe never write over each other: an operating-system lock on
// planification's own `.ktw/.holds.lock`, taken through the shared core/oslock
// package (the scheduler's store keeps its own, separate lock), held for as
// long as its holder lives. A live holder keeps it however long it works - its age
// takes nothing from it - and a dead one's is free at once, released by the
// system. No process removes the file: it stays, its note naming its last
// holder.
func holdsLocked(dir string, fn func() error) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, holdsLockFile)
	deadline := time.Now().Add(holdsLockWait)
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
			return fmt.Errorf("the holds are locked by %s (%s)", oslock.Note(path), path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
