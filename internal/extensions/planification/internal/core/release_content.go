// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WHAT A RELEASE HOLDS — increments and direct work, never a goal or a feature
// directly (they reach it through an increment); its level says what that
// content is: patch (direct work only), minor (increments), major (a breaking
// increment among them) — a lint warns of a mismatch, the freeze refuses it.
// Its candidates are deliveries of the one release; its deliverables are the
// modules its content touched and its change-log page.

// checkLevel refuses a level that is none of the three.
func checkLevel(verb, level string) error {
	if level != "" && !slices.Contains([]string{api.LevelMajor, api.LevelMinor, api.LevelPatch}, level) {
		return invalid("%s: level %q is major, minor or patch", verb, level)
	}
	return nil
}

// notIncrementWork reports what a release never holds directly: a goal or a
// feature (an increment holds them).
func notIncrementWork(o cwmapi.Object) bool {
	return o.Type == "goal" || o.Type == "feature"
}

// incrementsIn is the increments shipping in a release.
func incrementsIn(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object) ([]cwmapi.Object, error) {
	ids, err := incoming(ctx, cwm, rel.ID, api.EdgeShipsIn)
	if err != nil {
		return nil, err
	}
	var out []cwmapi.Object
	for _, id := range ids {
		o, _, err := loadAny(ctx, cwm, id)
		if err != nil {
			return nil, err
		}
		if o.Type == api.IncrementType && releaseState(o) != api.IncrementDropped {
			out = append(out, o)
		}
	}
	return out, nil
}

// levelMismatch says why a release's content does not fit its level; "" when
// it fits, or the release states none.
func levelMismatch(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object) (string, error) {
	level := releaseFields(rel).Level
	if rel.Type != api.ReleaseType || level == "" {
		return "", nil
	}
	incs, err := incrementsIn(ctx, cwm, rel)
	if err != nil {
		return "", err
	}
	breaking := slices.ContainsFunc(incs, func(o cwmapi.Object) bool { return releaseFields(o).Breaking })
	switch {
	case level == api.LevelPatch && len(incs) > 0:
		return fmt.Sprintf("%s is a patch: it holds direct work only, and %d increment(s) ship in it - make it a minor", rel.ID, len(incs)), nil
	case level == api.LevelMinor && len(incs) == 0:
		return fmt.Sprintf("%s is a minor: it holds increments, and none ships in it - make it a patch", rel.ID), nil
	case level == api.LevelMinor && breaking:
		return fmt.Sprintf("%s is a minor, and a breaking increment ships in it - make it a major", rel.ID), nil
	case level == api.LevelMajor && !breaking:
		return fmt.Sprintf("%s is a major: no breaking increment ships in it - make it a minor", rel.ID), nil
	}
	return "", nil
}

// Deliver records a delivery of a frozen or released release: a candidate.
func (r *Releases) Deliver(ctx context.Context, in api.DeliverInput) (api.ReleaseOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.ReleaseOutput{}, err
	}
	name := strings.TrimSpace(in.Name)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	o, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	if o.Type != api.ReleaseType {
		return api.ReleaseOutput{}, invalid("Deliver: %s is an increment - its release is delivered", in.ID)
	}
	if st := releaseState(o); st != api.ReleaseFrozen && st != api.ReleaseReleased {
		return api.ReleaseOutput{}, invalid("Deliver: %s is %s - a candidate is a delivery of a frozen release", in.ID, st)
	}
	f := releaseFields(o)
	if name == "" {
		// The next rcN: one above the highest, whatever was named by hand.
		n := 0
		for _, c := range f.Candidates {
			var k int
			if _, err := fmt.Sscanf(c.Name, "rc%d", &k); err == nil && k > n {
				n = k
			}
		}
		name = fmt.Sprintf("rc%d", n+1)
	}
	if slices.ContainsFunc(f.Candidates, func(c api.Candidate) bool { return c.Name == name }) {
		return api.ReleaseOutput{}, invalid("Deliver: %s already has a candidate %s", in.ID, name)
	}
	cand := api.Candidate{Name: name, At: time.Now().UTC().Format(time.RFC3339), By: in.By, Note: strings.TrimSpace(in.Note),
		Build: strings.TrimSpace(in.Build)}
	// Each delivery writes its letter, its « Quoi de neuf », from the
	// release's note; the sign-off approves it.
	if note, err := noteOf(ctx, cwm, o); err == nil {
		cand.Letter = letterOf(titleOf(o), f.Version, name, note)
	}
	f.Candidates = append(f.Candidates, cand)
	out, err := setFields(ctx, cwm, o, f)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	return api.ReleaseOutput{Release: toRelease(out)}, nil
}

// Deliverables answers what a release ships: the modules its direct work and
// its increments' work touched, and its change-log page.
func (r *Releases) Deliverables(ctx context.Context, in api.GetInput) (api.DeliverablesOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.DeliverablesOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.DeliverablesOutput{}, err
	}
	seen := map[string]bool{}
	add := func(id string) {
		if m, err := cwmapi.ModuleOfObjectID(id); err == nil && m != "" {
			seen[m] = true
		}
	}
	scope, err := scopeOf(ctx, cwm, rel)
	if err != nil {
		return api.DeliverablesOutput{}, err
	}
	for _, t := range scope.Targets {
		add(t.ID)
		serving, err := incoming(ctx, cwm, t.ID, api.EdgeServes)
		if err != nil {
			return api.DeliverablesOutput{}, err
		}
		for _, s := range serving {
			add(s)
		}
	}
	out := api.DeliverablesOutput{Release: rel.ID, Modules: []string{}}
	for m := range seen {
		out.Modules = append(out.Modules, m)
	}
	sort.Strings(out.Modules)
	if v := releaseFields(rel).Version; v != "" {
		out.ChangeLog = "docs/confluence-tew/changelog/" + v + ".md"
	}
	return out, nil
}
