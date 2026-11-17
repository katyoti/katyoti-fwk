// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sort"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// DeckTargets answers what the deck can be about: every module, each module
// (the system modules left out), and each open release.
func (v *Views) DeckTargets(ctx context.Context, _ api.DeckTargetsInput) (api.DeckTargetsOutput, error) {
	out := api.DeckTargetsOutput{Targets: []api.DeckTarget{{Value: "", Label: "All modules"}}}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return out, err
	}
	mods, err := cwmapi.UserModules(ctx, cwm)
	if err != nil {
		return out, err
	}
	var modules []api.DeckTarget
	for _, o := range mods {
		label := o.Name
		if label == "" {
			label = o.ID
		}
		modules = append(modules, api.DeckTarget{Value: o.ID, Label: label, Group: "Modules"})
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Label < modules[j].Label })
	out.Targets = append(out.Targets, modules...)
	rels, err := NewReleases().List(ctx, api.ListInput{})
	if err != nil {
		return out, err
	}
	var open []api.Release
	for _, r := range rels.Releases {
		if r.State == api.ReleasePlanned || r.State == api.ReleaseFrozen {
			open = append(open, r)
		}
	}
	sort.Slice(open, func(i, j int) bool {
		if open[i].Date != open[j].Date {
			return open[i].Date < open[j].Date
		}
		return open[i].ID < open[j].ID
	})
	for _, r := range open {
		label := r.Name
		if r.Date != "" {
			label += " — due " + r.Date
		}
		out.Targets = append(out.Targets, api.DeckTarget{Value: r.ID, Label: label, Group: "Releases"})
	}
	return out, nil
}

// inScopePlans keeps the plans a release targets, and the parts split from them.
func inScopePlans(ps []api.PlannedObject, scope map[string]bool) []api.PlannedObject {
	var out []api.PlannedObject
	for _, p := range ps {
		if scope[p.ID] || (p.SplitFrom != "" && scope[p.SplitFrom]) {
			out = append(out, p)
		}
	}
	return out
}

// inScopeDemands keeps the demand trees whose root a release targets.
func inScopeDemands(ds []api.Demand, scope map[string]bool) []api.Demand {
	var out []api.Demand
	for _, d := range ds {
		if scope[d.ID] {
			out = append(out, d)
		}
	}
	return out
}
