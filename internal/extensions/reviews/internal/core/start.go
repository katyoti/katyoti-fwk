// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// Start runs the review types chosen on in.Object — by name, preset or
// dimension; none named: every manual type that applies. A manual run whose
// estimate (the types' cost ceilings) is over api.ConfirmOverUSD is only
// estimated until it is confirmed; an automatic run leaves out a type with no
// ceiling. A check type runs now and is filed; an agent type starts the
// review-run task (the agent follows the type's skill, then files).
func (s *Service) Start(ctx context.Context, in api.StartInput) (api.StartOutput, error) {
	if in.Object == "" {
		return api.StartOutput{}, fmt.Errorf("%w: object required", cwmapi.ErrInvalidInput)
	}
	trigger := in.Trigger
	if trigger == "" {
		trigger = api.TriggerManual
	}
	avail, err := s.TypesFor(ctx, api.TypesInput{ID: in.Object})
	if err != nil {
		return api.StartOutput{}, err
	}
	chosen := choose(avail, in, trigger)
	if len(chosen) == 0 && trigger == api.TriggerAutomatic {
		return api.StartOutput{}, nil // no automatic review applies to this change
	}
	if len(chosen) == 0 {
		return api.StartOutput{}, fmt.Errorf("%w: no review type of %s matches the choice", cwmapi.ErrInvalidInput, in.Object)
	}
	out := api.StartOutput{}
	for _, t := range chosen {
		out.EstimateUSD += t.MaxCostUSD
	}
	if trigger == api.TriggerManual && out.EstimateUSD > api.ConfirmOverUSD && !in.Confirm {
		out.NeedsConfirm = true
		return out, nil
	}
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil {
		return out, fmt.Errorf("reviews: no ActionService: %w", err)
	}
	out.Run = "run-" + time.Now().UTC().Format("20060102T150405Z")
	provider, _ := application.SelfProviderFromContext(ctx)
	for _, t := range chosen {
		if trigger == api.TriggerAutomatic && t.Trigger.Owners != "" && len(in.Paths) > 0 {
			out.Started = append(out.Started, s.runOnOwners(ctx, actions, provider, t, in, trigger, out.Run)...)
			continue
		}
		out.Started = append(out.Started, s.runOne(ctx, actions, provider, t, in, trigger, out.Run))
	}
	return out, nil
}

// runOne runs one type on in.Object: a check is filed at once, an agent type
// starts its run.
func (s *Service) runOne(ctx context.Context, actions actionsapi.ActionService, provider string, t api.ReviewTypeDef, in api.StartInput, trigger, run string) api.StartedType {
	st := api.StartedType{Type: t.Name}
	var err error
	if t.Runner.Check != "" {
		var filed api.FileOutput
		filed, err = s.runCheck(ctx, actions, t, in, trigger, run)
		st.Review, st.Files, st.Skipped = filed.Review, filed.Files, filed.Skipped
	} else {
		st.TaskID, err = runAgent(ctx, actions, provider, t, in, trigger, run)
	}
	if err != nil {
		st.Error = err.Error()
	}
	return st
}

// runOnOwners runs a type whose trigger names an owners action on each object
// owning the change's paths, with its own paths: a change crossing modules is
// reviewed in each, and an object owning none of them is not reviewed (a run
// that checked nothing would fold its findings away). An owner the type does
// not apply to is left out.
func (s *Service) runOnOwners(ctx context.Context, actions actionsapi.ActionService, provider string, t api.ReviewTypeDef, in api.StartInput, trigger, run string) []api.StartedType {
	input, _ := json.Marshal(api.ChangeOwnersInput{Paths: in.Paths})
	got, err := actions.Run(ctx, actionsapi.RunInput{URI: t.Trigger.Owners, Input: input})
	var owners api.ChangeOwnersOutput
	if err == nil {
		err = json.Unmarshal(got.Value, &owners)
	}
	if err != nil {
		return []api.StartedType{{Type: t.Name, Error: fmt.Sprintf("the owners of the change (%s): %v", t.Trigger.Owners, err)}}
	}
	var out []api.StartedType
	for _, o := range owners.Owners {
		if o.Object == "" || len(o.Paths) == 0 || !s.applies(ctx, t.Name, o.Object) {
			continue
		}
		one := in
		one.Object, one.Paths = o.Object, o.Paths
		st := s.runOne(ctx, actions, provider, t, one, trigger, run)
		st.Object = o.Object
		out = append(out, st)
	}
	return out
}

// applies says whether the review type named applies to the object.
func (s *Service) applies(ctx context.Context, typ, object string) bool {
	avail, err := s.TypesFor(ctx, api.TypesInput{ID: object})
	if err != nil {
		return false
	}
	for _, d := range avail.Dimensions {
		for _, t := range d.Types {
			if t.Name == typ {
				return true
			}
		}
	}
	return false
}

// followsChange says whether an automatic type follows a request's change:
// its kinds hold the request's kind and one of its path globs matches a path
// the change touched (each empty: any). A run naming no kind or no paths is
// not narrowed by them.
func followsChange(tr api.ReviewTrigger, kind string, paths []string) bool {
	if kind != "" && len(tr.Kinds) > 0 && !slices.Contains(tr.Kinds, kind) {
		return false
	}
	if len(paths) == 0 || len(tr.Paths) == 0 {
		return true
	}
	for _, glob := range tr.Paths {
		for _, p := range paths {
			if ok, _ := path.Match(glob, p); ok {
				return true
			}
			if ok, _ := path.Match(glob, path.Base(p)); ok {
				return true
			}
		}
	}
	return false
}

// choose keeps the types the input names (by name, preset or dimension), or
// every manual one when it names none; an automatic run keeps only the
// automatic types with a cost ceiling.
func choose(avail api.TypesOutput, in api.StartInput, trigger string) []api.ReviewTypeDef {
	inPreset := map[string]bool{}
	for _, p := range avail.Presets {
		if p.Name == in.Preset {
			for _, n := range p.Types {
				inPreset[n] = true
			}
		}
	}
	named := len(in.Types) > 0 || in.Preset != "" || in.Dimension != ""
	var out []api.ReviewTypeDef
	for _, d := range avail.Dimensions {
		for _, t := range d.Types {
			mode := t.Trigger.Mode
			if mode == "" {
				mode = api.ModeManual
			}
			keep := slices.Contains(in.Types, t.Name) || inPreset[t.Name] || (in.Dimension != "" && t.Dimension == in.Dimension)
			if !named {
				keep = mode == trigger
			}
			if trigger == api.TriggerAutomatic && (mode != api.ModeAutomatic || t.MaxCostUSD <= 0) && t.Runner.Check == "" {
				keep = false
			}
			if trigger == api.TriggerAutomatic && !followsChange(t.Trigger, in.Kind, in.Paths) {
				keep = false
			}
			if keep {
				out = append(out, t)
			}
		}
	}
	return out
}

// runCheck calls the type's check with the object and files what it answers;
// it answers the review and the files the filing wrote or removed.
func (s *Service) runCheck(ctx context.Context, actions actionsapi.ActionService, t api.ReviewTypeDef, in api.StartInput, trigger, run string) (api.FileOutput, error) {
	check := map[string]any{"object": in.Object, "type": t.Name, "input": t.Runner.Input}
	if len(in.Paths) > 0 {
		check["paths"] = in.Paths // a change's review: the check scopes to what it touched
	}
	input, _ := json.Marshal(check)
	got, err := actions.Run(ctx, actionsapi.RunInput{URI: t.Runner.Check, Input: input})
	if err != nil {
		return api.FileOutput{}, err
	}
	var res api.ReviewResult
	if err := json.Unmarshal(got.Value, &res); err != nil {
		return api.FileOutput{}, fmt.Errorf("check %s answered no review result: %w", t.Runner.Check, err)
	}
	return s.File(ctx, api.FileInput{Object: in.Object, Type: t.Name, Run: run, Trigger: trigger,
		RequestedBy: in.RequestedBy, Commit: in.Commit, Reviewers: []string{t.Runner.Check}, Result: res})
}

// runAgent starts the review-run task for an agent type: the agent follows the
// type's skill on the object, answers a review result, and the task files it.
func runAgent(ctx context.Context, actions actionsapi.ActionService, provider string, t api.ReviewTypeDef, in api.StartInput, trigger, run string) (string, error) {
	if t.Runner.Skill == "" {
		return "", fmt.Errorf("review type %s names neither a skill nor a check", t.Name)
	}
	if provider == "" {
		provider = "ktw-agents"
	}
	input, _ := json.Marshal(map[string]any{
		"object": in.Object, "type": t.Name, "title": t.Title, "description": t.Description, "rating": t.Rating,
		"skill": t.Runner.Skill, "input": t.Runner.Input,
		"run": run, "trigger": trigger, "requested_by": in.RequestedBy, "commit": in.Commit,
	})
	got, err := actions.Run(ctx, actionsapi.RunInput{URI: provider + ":tasks/" + api.ReviewRunTask, Input: input,
		Context: map[string]any{"_target": in.Object}})
	if err != nil {
		return "", err
	}
	return got.TaskID, nil
}
