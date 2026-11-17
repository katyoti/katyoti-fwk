// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A RELEASE PLANS ITS WORK FROM THE GAP — nothing is planned until a release
// or an increment asks (OD-010): plan-work walks each increment shipping in the
// release, or the one increment (one of a periodic or on-change stream ships in
// no release until it is delivered). A goal not
// decomposed gets its decomposition (a request on the goal: its type-state
// template); the features it reaches (serving it, or under it) join the
// increment's, computed at each run. A feature done needs nothing; one not
// specified gets its specification, one ready or being built its build (a
// request on the feature), one realized whose criteria pass is verified at once
// or gets its verify round - one specified but not ready waits on its gate. A
// feature whose criteria pass but which is not done is not passing (OD-019). Open work is reused, never filed twice; two requests one
// run files in a module are ordered (the file-race rule). Run again, it files
// nothing new.

// PlanWork plans a release's work from the gap.
func (r *Releases) PlanWork(ctx context.Context, in api.ReleasePlanWorkInput) (api.ReleasePlanWorkOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.ReleasePlanWorkOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleasePlanWorkOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ReleasePlanWorkOutput{}, err
	}
	if rel.Type != api.ReleaseType {
		return api.ReleasePlanWorkOutput{}, invalid("PlanWork: %s is an increment - plan the release it ships in, or the increment itself (increments plan-work)", in.ID)
	}
	incs, err := incrementsIn(ctx, cwm, rel)
	if err != nil {
		return api.ReleasePlanWorkOutput{}, err
	}
	// Define now, plan when run (rule): an increment
	// defined by its delivery goals is planned once a person ran it - until
	// then it waits for its run; one holding its goals and features with no
	// delivery goals (aimed by hand) has nothing to run and is planned.
	var ran, defined []cwmapi.Object
	for _, o := range incs {
		if hasRequest(o) || len(releaseFields(o).Delivery) == 0 {
			ran = append(ran, o)
		} else {
			defined = append(defined, o)
		}
	}
	out, err := planIncrements(ctx, cwm, ran, in.By)
	if err != nil {
		return out, err
	}
	for _, o := range defined {
		out.Waiting = append(out.Waiting, api.GapItem{Object: o.ID, Why: "defined, not run: planned when a person runs it (increments run)"})
	}
	return out, nil
}

// PlanWork plans one open increment's work from the gap, whether it ships in
// a release or not: an increment of a periodic or on-change stream has none
// until it is delivered, and its work is planned all the same.
func (i *Increments) PlanWork(ctx context.Context, in api.IncrementPlanWorkInput) (api.ReleasePlanWorkOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.ReleasePlanWorkOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleasePlanWorkOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.ReleasePlanWorkOutput{}, err
	}
	if st := releaseState(o); !open(st) {
		return api.ReleasePlanWorkOutput{}, invalid("PlanWork: %s is %s", in.ID, st)
	}
	if why := definedNotRun(o); why != "" {
		return api.ReleasePlanWorkOutput{}, invalid("PlanWork: %s", why)
	}
	return planIncrements(ctx, cwm, []cwmapi.Object{o}, in.By)
}

// planIncrements walks the increments given: what each holds is decomposed,
// specified or built from the gap.
func planIncrements(ctx context.Context, cwm cwmapi.CWMService, incs []cwmapi.Object, by cwmapi.Actor) (api.ReleasePlanWorkOutput, error) {
	// A feature passing its acceptance criteria needs no work. Planned from an
	// apply's collect - the report waits for it - no acceptance probe runs:
	// a feature recorded done passes, every other one gets its work, whose own
	// verification finds one that already passes (
	// decision OD-017). Asked directly (plan-work), the probes run.
	passes := func(o cwmapi.Object) bool { return lifeState(o) == "done" }
	if !collecting(ctx) {
		verification, err := application.UseService[verificationapi.VerificationService](ctx)
		if err != nil || verification == nil {
			return api.ReleasePlanWorkOutput{}, fmt.Errorf("planification.PlanWork: no VerificationService: %v", err)
		}
		passes = func(o cwmapi.Object) bool { return gradeMember(ctx, cwm, verification, o.ID).Status == api.MemberPass }
	}
	out := api.ReleasePlanWorkOutput{Filed: []api.GapItem{}, Reused: []api.GapItem{}, Passing: []api.GapItem{}, Waiting: []api.GapItem{}}
	lastIn := map[string]string{} // the last request this run filed, per module
	svc := New()
	// A goal's follow (recipe goal-follow) waits on the features serving it
	// and ends with them: it is never in the chain - a feature depending on
	// it would wait on what waits on it - and depends on each feature's work
	// instead. A goal's decomposition stays in it.
	follows := map[string]bool{}
	// chainAs files a request; chained, it depends on the last one this run
	// filed in its module and becomes the one the next depends on.
	// isFollow says a goal's request is its follow (recipe goal-follow).
	isFollow := func(id string) bool {
		o, err := loadObject(ctx, cwm, id)
		if err != nil || o.Type != goalType || !hasRequest(o) {
			return false
		}
		t, err := templateFor(ctx, o)
		return err == nil && t.Name == goalFollowRecipe
	}
	chainAs := func(in api.FileInput, why string, chained bool) error {
		got, err := svc.File(ctx, in)
		if err != nil {
			return fmt.Errorf("plan-work on %s: %w", in.On, err)
		}
		out.Filed = append(out.Filed, api.GapItem{Object: in.On, Request: got.Request.ID, Why: why})
		if chained && isFollow(got.Request.ID) {
			follows[got.Request.ID] = true
			chained = false
		}
		if !chained {
			return nil
		}
		module, _ := cwmapi.ModuleOfObjectID(in.On)
		if prev := lastIn[module]; prev != "" {
			if _, err := svc.Depend(ctx, api.DependInput{ID: got.Request.ID, On: prev}); err != nil {
				return err
			}
		}
		lastIn[module] = got.Request.ID
		return nil
	}
	fileAs := func(in api.FileInput, why string) error { return chainAs(in, why, true) }
	file := func(o cwmapi.Object, why string) error {
		return chainAs(api.FileInput{On: o.ID, By: by}, why, true)
	}
	// fileVerifyRound files a realized feature's verify round
	// (feature-verify) in the increment: an agent reads its code against its
	// criteria and moves it done, or reopens it with what is missing.
	fileVerifyRound := func(o cwmapi.Object, inc string) error {
		return fileAs(api.FileInput{
			On:        o.ID,
			Kind:      api.KindReview,
			Title:     "Verify the realized feature",
			Details:   "Its acceptance criteria pass but it is not done, and an agent may not verify it at once (no criterion of its own, a criterion with no passing verdict, or a deliverable tested by hand). Verify it is really implemented - run its acceptance criteria, read its code - then move it done (transition verify) or send it back (transition reopen) with what is missing, and file that work.",
			Increment: inc,
			By:        by,
		}, "its criteria pass but it is not done: its verify round")
	}
	// openWork is the open request carrying o or serving it, or "".
	openWork := func(o cwmapi.Object) string {
		if hasRequest(o) && !planEnded(stateOf(o)) {
			return o.ID
		}
		serving, _ := incoming(ctx, cwm, o.ID, api.EdgeServes)
		for _, s := range serving {
			if so, err := loadObject(ctx, cwm, s); err == nil && hasRequest(so) && !planEnded(stateOf(so)) {
				return s
			}
		}
		return ""
	}
	seen := map[string]bool{}
	for _, inc := range incs {
		targets, err := workOf(ctx, cwm, inc.ID)
		if err != nil {
			return out, err
		}
		var features []string
		for _, id := range targets {
			o, err := loadObject(ctx, cwm, id)
			if err != nil {
				return out, err
			}
			switch o.Type {
			case "goal":
				// The person's approval of the proposal that added it is its
				// acceptance: a goal still idea, proposed or in review is
				// decomposed, as such a feature is specified.
				if slices.Contains([]string{"", "idea", "proposed", "in-review", "accepted"}, lifeState(o)) {
					if w := openWork(o); w != "" {
						out.Reused = append(out.Reused, api.GapItem{Object: o.ID, Request: w, Why: "its decomposition is open"})
						if isFollow(w) {
							follows[w] = true
						}
					} else if err := file(o, "a goal not decomposed: its decomposition"); err != nil {
						return out, err
					}
				}
				serving, err := incoming(ctx, cwm, o.ID, api.EdgeServes)
				if err != nil {
					return out, err
				}
				for _, f := range serving {
					if fo, err := loadObject(ctx, cwm, f); err == nil && fo.Type == "feature" {
						features = append(features, f)
					}
				}
				kids, err := childrenOf(ctx, cwm, o.ID, "feature")
				if err != nil {
					return out, err
				}
				for _, k := range kids {
					features = append(features, k.ID)
				}
			case "feature":
				features = append(features, o.ID)
			}
		}
		for _, id := range features {
			if seen[id] {
				continue
			}
			seen[id] = true
			o, err := loadObject(ctx, cwm, id)
			if err != nil {
				return out, err
			}
			// A feature whose criteria pass but which is not done is not
			// passing (decision): realized, it is
			// verified at once when an agent may
			// otherwise its end is planned like any other gap - its
			// specification, its build, or a realized one's verify round -
			// so no increment hangs on a green feature left not done.
			green := passes(o)
			if green && (lifeState(o) == "done" || verifyRealized(ctx, cwm, id)) {
				out.Passing = append(out.Passing, api.GapItem{Object: id, Why: "its acceptance criteria pass"})
				continue
			}
			if w := openWork(o); w != "" {
				out.Reused = append(out.Reused, api.GapItem{Object: id, Request: w, Why: "its work is open"})
				continue
			}
			st := lifeState(o)
			if st == "" {
				st = "idea" // just added: its workflow's first state
			}
			switch {
			case slices.Contains([]string{"accepted", "proposed", "idea"}, st):
				if err := file(o, "not specified: its specification"); err != nil {
					return out, err
				}
			case st == "ready" || st == "active":
				why := "not passing its criteria: its build"
				if green {
					why = "its criteria pass but it is not built to its end: its build"
				}
				if err := file(o, why); err != nil {
					return out, err
				}
			case st == featureRealized && green:
				if err := fileVerifyRound(o, inc.ID); err != nil {
					return out, err
				}
			default:
				out.Waiting = append(out.Waiting, api.GapItem{Object: id, Why: "state " + st + ": its gate is met first"})
			}
		}
	}
	// Each goal followed waits on the open work of its features.
	for g := range follows {
		for _, f := range featuresOfGoal(ctx, cwm, g) {
			fo, err := loadObject(ctx, cwm, f)
			if err != nil {
				continue
			}
			if w := openWork(fo); w != "" && w != g {
				if _, err := svc.Depend(ctx, api.DependInput{ID: g, On: w}); err != nil {
					return out, err
				}
			}
		}
	}
	return out, nil
}

// goalFollowRecipe is a goal's follow: it builds nothing and ends with the
// features serving it.
const goalFollowRecipe = "goal-follow"
