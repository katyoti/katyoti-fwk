// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

// ONLY THE RELEASE BEING BUILT AND THE NEXT ONE ARE OPEN (rule
// , define now - plan when run): a stream holds at most two
// open releases - planned or frozen - and nothing is planned for an increment
// defined by its delivery goals until a person runs it. Each refusal names the
// rule; a release in no stream counts in none.

// maxOpenReleases is how many releases a stream keeps open: the one being
// built and the next.
const maxOpenReleases = 2

// ruleDefineRun is the rule each refusal below names.
const ruleDefineRun = "rule: define now - plan when run"

// w5Exception is the field of a release that records a person's exception to
// the two open releases of its stream (rule, W5): {by:
// {id, kind: person}, at, note}. A release carrying it counts in none of its
// stream's two: it is created, takes increments and is planned for beside
// them.
const w5Exception = "w5_exception"

// personsException says whether a release carries a person's exception to
// the rule: its w5_exception set by a person - one an agent set is none.
func personsException(o cwmapi.Object) bool {
	x, ok := o.Fields[w5Exception].(map[string]any)
	if !ok {
		return false
	}
	by, ok := x["by"].(map[string]any)
	if !ok {
		return false
	}
	id, _ := by["id"].(string)
	kind, _ := by["kind"].(string)
	return strings.TrimSpace(id) != "" && profileapi.IsPerson(cwmapi.Actor{ID: id, Kind: kind})
}

// openRelease says whether a release still counts as open in its stream.
func openRelease(o cwmapi.Object) bool {
	st := releaseState(o)
	return st == api.ReleasePlanned || st == api.ReleaseFrozen
}

// streamOpenReleases is a stream's open releases that count toward its two -
// one carrying the person's exception counts in none - the one being built
// first: by date, a release with none last, then by id.
func streamOpenReleases(ctx context.Context, cwm cwmapi.CWMService, stream string) ([]cwmapi.Object, error) {
	rels, err := inStream(ctx, cwm, stream, api.ReleaseType)
	if err != nil {
		return nil, err
	}
	var out []cwmapi.Object
	for _, r := range rels {
		if openRelease(r) && !personsException(r) {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := releaseFields(out[i]).Date, releaseFields(out[j]).Date
		switch {
		case di == dj:
			return out[i].ID < out[j].ID
		case di == "":
			return false
		case dj == "":
			return true
		}
		return di < dj
	})
	return out, nil
}

// noRoomInStream says why a stream takes no new open release - "" when it
// takes one: it holds its two already.
func noRoomInStream(ctx context.Context, cwm cwmapi.CWMService, stream string) (string, error) {
	if stream == "" {
		return "", nil
	}
	rels, err := streamOpenReleases(ctx, cwm, stream)
	if err != nil {
		return "", err
	}
	var held []string
	for _, r := range rels {
		held = append(held, r.ID+" "+releaseFields(r).Version)
	}
	if len(held) < maxOpenReleases {
		return "", nil
	}
	return "stream " + stream + " has its open releases already (" + strings.Join(held, ", ") +
		"): only the release being built and the next one are open (" + ruleDefineRun +
		") - release or drop one first, or a person records their exception (fields.w5_exception) on a release placed in the stream", nil
}

// beyondOpenTwo says why a release takes no increment - "" when it takes
// one: an open release of a stream that is not among its first two (only data
// from before the rule holds a third).
func beyondOpenTwo(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object) (string, error) {
	stream := edgeTo(rel, api.EdgeInStream)
	if stream == "" || !openRelease(rel) || personsException(rel) {
		return "", nil
	}
	rels, err := streamOpenReleases(ctx, cwm, stream)
	if err != nil {
		return "", err
	}
	for i, r := range rels {
		if r.ID == rel.ID && i >= maxOpenReleases {
			return rel.ID + " is a third open release of " + stream + ": only the release being built and the next one take increments (" +
				ruleDefineRun + ") - release or drop one first, or a person records their exception on it (" + w5Exception + ")", nil
		}
	}
	return "", nil
}

// definedNotRun says why nothing is planned for an increment yet - "" when
// its work may be planned: one defined by its delivery goals is planned once
// a person ran it; one holding goals and features with no delivery goals
// (aimed by hand) has nothing to run.
func definedNotRun(o cwmapi.Object) string {
	if o.Type != api.IncrementType || hasRequest(o) || len(releaseFields(o).Delivery) == 0 {
		return ""
	}
	return o.ID + " is defined, not run: nothing is planned for it until a person runs it (increments run; " +
		ruleDefineRun + ") - urgent work is a request of its own, outside it"
}
