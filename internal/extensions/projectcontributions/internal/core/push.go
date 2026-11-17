// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
)

// Pusher is the publisher of object-backed entries in the process hosting
// CWM (decision D1): on each object event it projects the object with
// api.EntryOfObject and sends a put - or a remove when the object publishes
// nothing any more - under its project's provider token; Snapshot sends a
// project's whole set. The hub's project member applies them
// (contrib.Member); the hub never reads CWM.
type Pusher struct {
	send  func(ctx context.Context, p api.EntriesPush) error
	first func(ctx context.Context)
	once  sync.Once

	mu     sync.Mutex
	tokens map[string]string  // by project key and module: the last token resolved
	files  map[string]watched // by project key and object: the module files its layers read
}

// watched is an object whose entry reads files of its module (files/<path>
// layers): what it was pushed from, to push it again when one changes.
type watched struct {
	key, object, module string
	stamps              map[string]string // by module-relative path: the file's stamp when read
}

// NewPusher sends each push through send; first runs once, with the first
// object event the process observes (the moment its CWM is open: the
// binary's wiring pushes a snapshot then and starts answering the hub's
// snapshot requests). first may be nil.
func NewPusher(send func(ctx context.Context, p api.EntriesPush) error, first func(ctx context.Context)) *Pusher {
	return &Pusher{send: send, first: first, tokens: map[string]string{}, files: map[string]watched{}}
}

// OnEvent is the CWM event observer (cwmapi.EventObserver.Handle): an
// object's created, updated, deleted, archived or moved event pushes what the
// object publishes now (a deleted or archived object, its removal). Every
// other event is ignored.
func (p *Pusher) OnEvent(ctx context.Context, ev cwmapi.EventEnvelope) {
	if p.first != nil {
		p.once.Do(func() { p.first(ctx) })
	}
	action := ev.Topic[strings.LastIndex(ev.Topic, ".")+1:]
	if !strings.Contains(ev.Topic, ".object.") {
		return
	}
	switch action {
	case "created", "updated", "deleted":
		var pl cwmapi.ObjectEventPayload
		if json.Unmarshal(ev.Payload, &pl) != nil || pl.ObjectID == "" {
			return
		}
		p.report(ctx, pl.ObjectID, pl.Module, action == "deleted")
	case cwmapi.ArchivedEventAction:
		// An archived object has left the index: its entry is withdrawn.
		var pl cwmapi.ObjectArchivedEventPayload
		if json.Unmarshal(ev.Payload, &pl) != nil || pl.ObjectID == "" {
			return
		}
		p.report(ctx, pl.ObjectID, pl.Module, true)
	case "moved":
		var pl cwmapi.ObjectMovedEventPayload
		if json.Unmarshal(ev.Payload, &pl) != nil || pl.ObjectID == "" {
			return
		}
		if pl.FromID != "" && pl.FromID != pl.ObjectID {
			p.report(ctx, pl.FromID, pl.FromModule, true)
		}
		p.report(ctx, pl.ObjectID, pl.Module, false)
	}
}

// report pushes what object publishes now: its entry, or its removal when
// gone or no longer carrying the capability.
func (p *Pusher) report(ctx context.Context, object, module string, gone bool) {
	if err := p.push(ctx, object, module, gone); err != nil {
		slog.Warn("project-contributions: an object's entry was not pushed", "object", object, "error", err)
	}
}

func (p *Pusher) push(ctx context.Context, object, module string, gone bool) error {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return err
	}
	key := cwmapi.EventProject(ctx)
	token, err := p.tokenOf(ctx, cwm, key, object, module)
	if err != nil || token == "" {
		return err
	}
	if !gone {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: qualified(key, object)})
		if err != nil {
			p.watch(key, object, module, nil)
			return err
		}
		if c, carries, _ := api.ContributesOf(got.Object); carries {
			// Stamped before the read: a write in between is seen by the
			// next look (WatchLayerFiles) and pushed again.
			p.watch(key, object, module, layerStamps(ctx, cwm, got.Object.ID, c))
			e, err := api.EntryOfObject(ctx, got.Object, token, cwm)
			if err != nil {
				// An entry that does not project is not served: its removal
				// withdraws the one served before.
				slog.Warn("project-contributions: an object's entry does not project; withdrawn", "object", object, "error", err)
			} else {
				return p.send(ctx, api.EntriesPush{Op: api.PushPut, Token: token, Object: object, Entry: &e})
			}
			// Still watched: writing the missing layer file serves it.
			return p.send(ctx, api.EntriesPush{Op: api.PushRemove, Token: token, Object: object})
		}
	}
	p.watch(key, object, module, nil)
	return p.send(ctx, api.EntriesPush{Op: api.PushRemove, Token: token, Object: object})
}

// WatchLayerFiles looks every interval, for as long as ctx lives, at the
// module files the pushed entries' layers read (files/<path>): writing one -
// ObjectFileWrite, a hand edit, a pull - publishes no object event, so the
// object whose file changed is pushed again here, and the hub serves the new
// bytes with no restart.
func (p *Pusher) WatchLayerFiles(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.RefreshLayerFiles(ctx)
		}
	}
}

// RefreshLayerFiles pushes again each watched object one of whose layer
// files changed (written, removed or created) since it was last pushed.
func (p *Pusher) RefreshLayerFiles(ctx context.Context) {
	p.mu.Lock()
	all := make([]watched, 0, len(p.files))
	for _, w := range p.files {
		all = append(all, w)
	}
	p.mu.Unlock()
	if len(all) == 0 {
		return
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return
	}
	for _, w := range all {
		id := qualified(w.key, w.object)
		for rel, stamp := range w.stamps {
			if fileStamp(ctx, cwm, id, rel) != stamp {
				p.report(cwmapi.WithEventProject(ctx, w.key), w.object, w.module, false)
				break
			}
		}
	}
}

// watch records what object's layers read (nil stamps: nothing, it is
// forgotten).
func (p *Pusher) watch(key, object, module string, stamps map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(stamps) == 0 {
		delete(p.files, key+"|"+object)
		return
	}
	p.files[key+"|"+object] = watched{key: key, object: object, module: module, stamps: stamps}
}

// layerStamps stamps each module file c's layers read (id is the object's,
// as CWM reads its files).
func layerStamps(ctx context.Context, cwm cwmapi.CWMService, id string, c api.Contributes) map[string]string {
	var out map[string]string
	for _, src := range c.Layers {
		rel, ok := strings.CutPrefix(src, api.SourceFiles)
		if !ok {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[rel] = fileStamp(ctx, cwm, id, rel)
	}
	return out
}

// fileStamp is a module file's size and time, "" when it does not read.
func fileStamp(ctx context.Context, cwm cwmapi.CWMService, id, rel string) string {
	st, err := cwm.ObjectFileStat(ctx, cwmapi.ObjectFileStatInput{ID: id, Path: rel})
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d|%d", st.Entry.SizeBytes, st.Entry.ModTime.UnixNano())
}

// Snapshot sends the whole set of object entries of every project the hub
// serves (one push per member token, api.ProjectMembers; a member with none
// sends an empty set) - an object of a project the hub refuses (its token
// already held by an earlier project, reserved) or does not serve (no
// contributions folder) is not sent, never under another project's token.
// With no project attached, the CWM's own project is sent under its token.
// only, when not empty, keeps that token's project alone (a
// hub's request names it) - and sends nothing when this process does not
// serve it (neither an attached project's token nor a listed object's): an
// empty set would wipe the entries another process serves for it.
func (p *Pusher) Snapshot(ctx context.Context, only string) error {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return err
	}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: api.CapContributes, Limit: -1})
	if err != nil {
		return err
	}
	sets := map[string][]api.ObjectEntry{}
	served := servedTokens(attachedRoots())
	// Each served project's set is sent even empty: it clears what the hub
	// held of objects that lost the capability while no one pushed.
	for _, token := range served {
		if only == "" || token == only {
			sets[token] = nil
		}
	}
	for _, listed := range list.Objects {
		token, err := projectToken(ctx, cwm, listed.ID, served)
		if err != nil || token == "" || (only != "" && token != only) {
			continue
		}
		// A list holds the index's metadata: the object is read whole.
		module, _ := cwmapi.ModuleOfObjectID(listed.ID)
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: listed.ID})
		if err != nil {
			continue
		}
		o := got.Object
		if c, _, _ := api.ContributesOf(o); len(c.Layers) > 0 {
			p.watch("", listed.ID, module, layerStamps(ctx, cwm, o.ID, c))
		}
		e, err := api.EntryOfObject(ctx, o, token, cwm)
		if err != nil {
			slog.Warn("project-contributions: an object's entry does not project; not served", "object", o.ID, "error", err)
			continue
		}
		sets[token] = append(sets[token], e)
	}
	var errs []string
	for token, entries := range sets {
		if err := p.send(ctx, api.EntriesPush{Op: api.PushSnapshot, Token: token, Entries: entries}); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("project-contributions snapshot: %s", strings.Join(errs, "; "))
	}
	return nil
}

// tokenOf is the provider token of the project holding object (of module):
// the object's, else its module's, else the one last seen for that module (a
// deleted object no longer resolves), else - one project attached, and only
// then - the attached project's: with several, an object that does not
// resolve is not pushed under another project's token. An object of a
// project the hub does not serve is not pushed (servedTokens).
func (p *Pusher) tokenOf(ctx context.Context, cwm cwmapi.CWMService, key, object, module string) (string, error) {
	cacheKey := key + "|" + module
	roots := attachedRoots()
	served := servedTokens(roots)
	for _, ref := range []string{object, module} {
		if ref == "" {
			continue
		}
		token, err := projectToken(ctx, cwm, qualified(key, ref), served)
		if errors.Is(err, errNotServed) {
			slog.Warn("project-contributions: an object's project is not served by the hub; not pushed", "object", object, "module", module, "error", err)
			return "", nil
		}
		if err == nil && token != "" {
			p.mu.Lock()
			p.tokens[cacheKey] = token
			p.mu.Unlock()
			return token, nil
		}
	}
	p.mu.Lock()
	token := p.tokens[cacheKey]
	p.mu.Unlock()
	if token != "" || key != "" {
		return token, nil
	}
	if token := fallbackToken(roots, served); token != "" {
		return token, nil
	}
	slog.Warn("project-contributions: an object's project does not resolve and several projects are attached; not pushed", "object", object, "module", module)
	return "", nil
}

// attachedRoots is the universe's attached projects that exist on disk (a
// variable: a test attaches its own).
var attachedRoots = func() []string {
	var out []string
	for _, root := range cenv.AttachedProjects(application.Env().HomeDir()) {
		if root == "" {
			continue
		}
		if fi, err := os.Stat(root); err == nil && fi.IsDir() {
			out = append(out, root)
		}
	}
	return out
}

// errNotServed is the refusal of an object whose project the hub does not
// serve.
var errNotServed = errors.New("the project is not served by the hub")

// servedTokens is the provider token of each project of roots the hub serves
// - api.ProjectMembers, the one reader of who is served: a project with no
// contributions folder, a reserved token or a token an earlier project holds
// gives none - keyed by its root (rootKey). nil when no project is attached:
// the CWM's own project is then the one pushed. The universe's declared
// members are the hub's to read; a project named like one is refused there,
// and a push for a token no member answers to changes nothing.
func servedTokens(roots []string) map[string]string {
	if len(roots) == 0 {
		return nil
	}
	members, _ := api.ProjectMembers(roots, nil)
	out := make(map[string]string, len(members))
	for _, m := range members {
		out[rootKey(m.Project)] = m.Token
	}
	return out
}

// fallbackToken is the token an object whose project does not resolve is
// pushed under: the attached project's when exactly one is attached and
// served, "" otherwise (none, or several: which one holds it is unknown).
func fallbackToken(roots []string, served map[string]string) string {
	if len(roots) != 1 {
		return ""
	}
	return served[rootKey(roots[0])]
}

// projectToken is the provider token of the project holding ref (an object
// or a module): when projects are attached (served not nil), the token the
// hub serves that project under, errNotServed when it serves none.
func projectToken(ctx context.Context, cwm cwmapi.CWMService, ref string, served map[string]string) (string, error) {
	r, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: ref})
	if err != nil {
		return "", err
	}
	if served != nil {
		token, ok := served[rootKey(r.ProjectRoot)]
		if !ok {
			return "", fmt.Errorf("%w: project %s", errNotServed, r.ProjectRoot)
		}
		return token, nil
	}
	f, err := projectfile.Read(r.ProjectRoot)
	if err != nil {
		return "", err
	}
	return f.Contributions.Provider, nil
}

// rootKey compares project roots: absolute and clean, case-folded where the
// file system ignores case.
func rootKey(root string) string {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	root = filepath.Clean(root)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	return root
}

func qualified(key, id string) string {
	if key == "" {
		return id
	}
	return cwmapi.QualifiedID(key, id)
}
