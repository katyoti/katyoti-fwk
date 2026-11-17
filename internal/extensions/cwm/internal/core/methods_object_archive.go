// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	cwmevents "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
)

// ObjectArchive ends an object. Its file is RENAMED, bytes unchanged (git sees
// a pure rename), into its module's `.ktw/_history/<yyyy-mm>/`, which no scan
// reads, and it leaves the index with its outbound edges. Unlike
// ObjectDelete, the links other objects have to it are KEPT: its id joins the
// ended set before it is dropped (ended.go), so they stay valid as ended links
// and the id is never minted again. No delete hook runs; one `archived` event
// says where the file went.
//
// Refused inside a change set and for an object with children. Idempotent: an
// id already archived answers AlreadyArchived, with no event.
func (s *Service) ObjectArchive(ctx stdctx.Context, in api.ObjectArchiveInput) (api.ObjectArchiveOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return api.ObjectArchiveOutput{}, fmt.Errorf("%w: ObjectArchive: id required", api.ErrInvalidInput)
	}
	segment, err := historySegment(in.Into, time.Now())
	if err != nil {
		return api.ObjectArchiveOutput{}, err
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectArchiveOutput{}, err
	}
	if _, indexed := ix.get(in.ID); !indexed {
		if path, ended := s.isEnded(in.ID); ended {
			return api.ObjectArchiveOutput{ID: in.ID, Path: path, AlreadyArchived: true}, nil
		}
		return api.ObjectArchiveOutput{}, api.ObjectNotFound(in.ID)
	}
	module, _ := ix.moduleOf(in.ID)
	if module == "" {
		return api.ObjectArchiveOutput{}, fmt.Errorf("%w: ObjectArchive: %q belongs to no module — the project object is never archived", api.ErrInvalidInput, in.ID)
	}
	if err := s.checkReactionWrites(ctx, module, "ObjectArchive"); err != nil {
		return api.ObjectArchiveOutput{}, err
	}
	if s.openSet(ctx) != nil {
		return api.ObjectArchiveOutput{}, fmt.Errorf("%w: ObjectArchive: %q: an archive is not staged — run it outside a reaction's change set", api.ErrInvalidInput, in.ID)
	}
	if kids := ix.childrenOf(in.ID); len(kids) > 0 {
		ids := make([]string, 0, len(kids))
		for _, k := range kids {
			ids = append(ids, k.ID)
		}
		sort.Strings(ids)
		return api.ObjectArchiveOutput{}, fmt.Errorf("%w: ObjectArchive: %q has %d children (%s) — move or archive them first",
			api.ErrReferenced, in.ID, len(ids), strings.Join(ids, ", "))
	}
	src, ok := ix.pathOf(in.ID)
	if !ok {
		return api.ObjectArchiveOutput{}, api.ObjectNotFound(in.ID)
	}
	moduleDir := s.moduleDirOf(module, src)
	if moduleDir == "" {
		return api.ObjectArchiveOutput{}, fmt.Errorf("%w: ObjectArchive: %q: its file %s is outside its module's folders", api.ErrInvalidInput, in.ID, s.relPath(src))
	}
	dstDir := filepath.Join(moduleDir, filepath.FromSlash(api.HistoryFolder), segment)
	dst := filepath.Join(dstDir, filepath.Base(src))
	owned := s.ownedFilePaths(ctx, in.ID)

	unlock := s.locks.lock(in.ID)
	if err := s.holdsStore(); err != nil {
		unlock()
		return api.ObjectArchiveOutput{}, err
	}
	prior, rev, err := readObjectFileRev(src)
	if err != nil {
		unlock()
		if errors.Is(err, fs.ErrNotExist) {
			return api.ObjectArchiveOutput{}, api.ObjectNotFound(in.ID)
		}
		return api.ObjectArchiveOutput{}, err
	}
	if in.Revision != "" && in.Revision != rev {
		unlock()
		return api.ObjectArchiveOutput{}, api.ObjectConflict(in.ID, in.Revision)
	}
	if _, err := os.Stat(dst); err == nil {
		unlock()
		return api.ObjectArchiveOutput{}, fmt.Errorf("%w: ObjectArchive: %s already exists", api.ErrAlreadyExists, s.relPath(dst))
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		unlock()
		return api.ObjectArchiveOutput{}, fmt.Errorf("cwm/core: archive object %q: %w", in.ID, err)
	}
	if err := os.Rename(src, dst); err != nil {
		unlock()
		return api.ObjectArchiveOutput{}, fmt.Errorf("cwm/core: archive object %q: %w", in.ID, err)
	}
	rel := s.relPath(dst)
	// Ended BEFORE dropped: a reader never sees the id neither indexed nor
	// ended, so no link to it reads as dangling in between.
	s.noteEnded(in.ID, rel)
	ix.drop(in.ID)
	unlock()

	// Its owned files (memory, comments) go with it, beside it.
	for _, p := range owned {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		to := filepath.Join(dstDir, filepath.Base(p))
		if _, err := os.Stat(to); err == nil {
			slog.Warn("cwm: an archived object's file was left in place: its name is taken in the history folder", "id", in.ID, "file", s.relPath(p))
			continue
		}
		if err := os.Rename(p, to); err != nil {
			slog.Warn("cwm: an archived object's file could not follow it", "id", in.ID, "file", s.relPath(p), "err", err)
		}
	}

	s.Events.PublishObjectArchived(cwmevents.ObjectArchivedPayload{
		EventMeta: s.eventMeta(ctx, rev),
		ObjectID:  in.ID, Module: module, Type: prior.Type, Parent: prior.ParentID(), Path: rel,
	})
	return api.ObjectArchiveOutput{ID: in.ID, Path: rel}, nil
}

// historySegment is the history folder an archive goes to: Into's segment
// (`.ktw/_history/<segment>`, nothing else), else the month of now (UTC).
func historySegment(into string, now time.Time) (string, error) {
	if into == "" {
		return now.UTC().Format("2006-01"), nil
	}
	seg, ok := strings.CutPrefix(filepath.ToSlash(into), api.HistoryFolder+"/")
	if !ok || seg == "" || strings.ContainsAny(seg, `/\`) || strings.Contains(seg, "..") || strings.HasPrefix(seg, ".") {
		return "", fmt.Errorf("%w: ObjectArchive: into %q: an archive goes to %s/<segment>", api.ErrInvalidInput, into, api.HistoryFolder)
	}
	return seg, nil
}

// moduleDirOf is the module folder holding path: the authored one or the
// runtime store's; "" when path is under neither.
func (s *Service) moduleDirOf(module, path string) string {
	for _, dir := range s.moduleDirs(module) {
		if rel, err := filepath.Rel(dir, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return dir
		}
	}
	return ""
}
