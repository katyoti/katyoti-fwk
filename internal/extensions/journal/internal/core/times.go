// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
)

// Times is when one object was created and last changed, derived — no object
// stores its times.
//
// The journal first: its records about the object, followed through its moves
// and splits like History. The newest record (any action) is when it last
// changed; the newest `created` record met is when it was created (a later
// re-creation of a deleted id is the object's creation, not the first one).
// What the journal does not answer — the object predates it, or was written
// outside CWM — is read, best effort, from git's log of the object's file: its
// last commit for the change, its first for the creation. A creation git dates
// after the journal's oldest record of the object is not one: the object was
// there before, so the oldest record stands in.
//
// With several projects served, the id is read in the journal of the project
// holding it (scopeOf): two projects holding one bare id keep their own times.
func (s *Service) Times(ctx context.Context, in api.TimesInput) (api.TimesOutput, error) {
	if in.ID == "" {
		return api.TimesOutput{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.TimesOutput{}, err
	}
	key, id := scopeOf(ctx, cwm, in.ID)
	var out api.TimesOutput
	var oldest time.Time
	follow := followed(id)
	// Every record is matched (follow must see the moves) and none is kept:
	// the times are read on the way.
	if _, err := s.query(ctx, key, api.QueryInput{}, func(r api.Record) bool {
		if !follow(r) {
			return false
		}
		if out.UpdatedAt.IsZero() {
			out.UpdatedAt = r.At // newest first
		}
		if out.CreatedAt.IsZero() && r.Action == api.ActionCreated {
			out.CreatedAt = r.At
		}
		oldest = r.At
		return false
	}); err != nil {
		return api.TimesOutput{}, err
	}
	if out.CreatedAt.IsZero() || out.UpdatedAt.IsZero() {
		first, last := gitTimes(ctx, qualify(key, id))
		if out.UpdatedAt.IsZero() {
			out.UpdatedAt = last
		}
		if out.CreatedAt.IsZero() {
			out.CreatedAt = first
			if !oldest.IsZero() && (first.IsZero() || first.After(oldest)) {
				out.CreatedAt = oldest
			}
		}
	}
	out.CreatedAt, out.UpdatedAt = utc(out.CreatedAt), utc(out.UpdatedAt)
	return out, nil
}

// gitTimes is the first and the last commit of the object's file (author
// dates, followed through renames); zero when the file is not under git or
// anything fails.
func gitTimes(ctx context.Context, id string) (first, last time.Time) {
	root, rel, ok := objectFile(ctx, id)
	if !ok {
		return time.Time{}, time.Time{}
	}
	logOut, err := gitRun(ctx, root, "log", "--follow", "--format=%aI", "--", rel)
	if err != nil {
		return time.Time{}, time.Time{}
	}
	lines := strings.Fields(string(logOut)) // newest first
	if len(lines) == 0 {
		return time.Time{}, time.Time{}
	}
	last, _ = time.Parse(time.RFC3339, lines[0])
	first, _ = time.Parse(time.RFC3339, lines[len(lines)-1])
	return first, last
}

func utc(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC()
}
