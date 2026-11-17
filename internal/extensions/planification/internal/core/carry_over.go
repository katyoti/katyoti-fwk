// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT NOT DONE BY ITS RELEASE'S DATE MOVES ON:
// it leaves the release - which ships without it - for its stream's next
// release, planned for it, its moved field saying why; with no later release
// it waits in the stream's backlog, saying so. The stream's daily timer moves
// what is past its date (CarryOver); a person releasing before the date is
// still refused while an increment is open.

// dueTimer is the name of a stream's daily timer.
const dueTimer = "release-due"

// CarryOver moves the increments not done of the stream's releases whose
// date has passed.
func (s *Streams) CarryOver(ctx context.Context, in api.CutInput) (api.CarryOverOutput, error) {
	out := api.CarryOverOutput{Moved: []string{}}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return out, err
	}
	so, err := loadStream(ctx, cwm, in.ID)
	if err != nil {
		return out, err
	}
	rels, err := inStream(ctx, cwm, so.ID, api.ReleaseType)
	if err != nil {
		return out, err
	}
	today := time.Now().Format("2006-01-02")
	for _, r := range rels {
		st, date := releaseState(r), releaseFields(r).Date
		if (st != api.ReleasePlanned && st != api.ReleaseFrozen) || date == "" || date >= today {
			continue
		}
		moved, err := moveOnUndone(ctx, cwm, r, "not done by "+titleOf(r)+"'s date ("+date+")")
		if err != nil {
			return out, err
		}
		out.Moved = append(out.Moved, moved...)
	}
	if len(out.Moved) == 0 {
		out.Note = "no release of this stream is past its date with an increment not done"
	}
	return out, nil
}

// moveOnUndone moves each increment shipping in rel and not done to the
// stream's release after it, saying why.
func moveOnUndone(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, why string) ([]string, error) {
	ids, err := incoming(ctx, cwm, rel.ID, api.EdgeShipsIn)
	if err != nil {
		return nil, err
	}
	var moved []string
	stream := edgeTo(rel, api.EdgeInStream)
	next := ""
	if stream != "" {
		next = releaseAfter(ctx, cwm, stream, rel)
	}
	for _, id := range ids {
		o, _, err := loadAny(ctx, cwm, id)
		if err != nil || o.Type != api.IncrementType {
			continue
		}
		if st := releaseState(o); st == api.IncrementDone || st == api.IncrementDropped {
			continue
		}
		o = withEdge(o, api.EdgeShipsIn, "")
		say := "moved out of " + titleOf(rel) + ": " + why
		if next != "" {
			o = withEdge(o, api.EdgePlannedFor, next)
			say += " - planned for " + titleOfID(ctx, cwm, next)
		} else {
			say += " - no later release of its stream: it waits in the backlog"
		}
		f := releaseFields(o)
		f.Moved = say
		if err := o.SetCapability(api.CapRelease, f); err != nil {
			return moved, err
		}
		module, _ := cwmapi.ModuleOfObjectID(o.ID)
		if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o}); err != nil {
			return moved, fmt.Errorf("carry %s over: %w", o.ID, err)
		}
		moved = append(moved, o.ID)
	}
	if len(moved) > 0 {
		if err := settleAll(ctx, PlanningActor); err != nil {
			return moved, err
		}
	}
	return moved, nil
}

// releaseAfter is the stream's planned release that comes after rel - by
// date, then version - or "" when none does.
func releaseAfter(ctx context.Context, cwm cwmapi.CWMService, stream string, rel cwmapi.Object) string {
	rels, err := inStream(ctx, cwm, stream, api.ReleaseType)
	if err != nil {
		return ""
	}
	var later []api.Release
	for _, r := range rels {
		if r.ID == rel.ID || releaseState(r) != api.ReleasePlanned {
			continue
		}
		later = append(later, toRelease(r))
	}
	if len(later) == 0 {
		return ""
	}
	at := releaseFields(rel).Date
	sort.SliceStable(later, func(i, j int) bool {
		di, dj := later[i].Date, later[j].Date
		if (di == "") != (dj == "") {
			return di != ""
		}
		return di < dj
	})
	for _, r := range later {
		if at == "" || r.Date == "" || strings.Compare(r.Date, at) >= 0 {
			return r.ID
		}
	}
	return ""
}
