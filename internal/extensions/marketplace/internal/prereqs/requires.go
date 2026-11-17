// SPDX-License-Identifier: Apache-2.0

package prereqs

import (
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// SumRequires is the programs, keys and logins of several catalog items as
// one: what a solution needs is the sum of its members'. Each program, key
// and login appears once (programs by name, case-insensitive), required when
// any member requires it, at the highest version a member declares, with
// every declaring sidecar and every distinct recipe. Members and Formats are
// not summed: they belong to the item itself. Sorted by name.
func SumRequires(rs ...mktapi.Requires) mktapi.Requires {
	var out mktapi.Requires
	tools := map[string]int{}
	recipeSeen := map[string]map[string]bool{}
	keys := map[string]int{}
	logins := map[string]int{}
	for _, r := range rs {
		for _, t := range r.Tools {
			key := strings.ToLower(strings.TrimSpace(t.Name))
			if key == "" {
				continue
			}
			i, seen := tools[key]
			if !seen {
				i = len(out.Tools)
				tools[key] = i
				recipeSeen[key] = map[string]bool{}
				out.Tools = append(out.Tools, mktapi.ToolNeed{Name: t.Name, InstallSkill: t.InstallSkill})
			}
			o := &out.Tools[i]
			o.Required = o.Required || t.Required
			if CompareVersions(t.Version, o.Version) > 0 {
				o.Version = t.Version
			}
			if o.InstallSkill == "" {
				o.InstallSkill = t.InstallSkill
			}
			o.Commands = union(o.Commands, t.Commands)
			o.Platforms = union(o.Platforms, t.Platforms)
			o.NeededBy = union(o.NeededBy, t.NeededBy)
			for _, rc := range t.Recipes {
				k := recipeKey(rc)
				if rc.Kind == publish.RecipeScript {
					k += "|" + strings.Join(t.NeededBy, ",") // each sidecar ships its own script
				}
				if recipeSeen[key][k] {
					continue
				}
				recipeSeen[key][k] = true
				o.Recipes = append(o.Recipes, rc)
				o.Platforms = union(o.Platforms, []string{rc.Platform})
			}
		}
		for _, k := range r.APIKeys {
			if k.Name == "" {
				continue
			}
			i, seen := keys[k.Name]
			if !seen {
				i = len(out.APIKeys)
				keys[k.Name] = i
				out.APIKeys = append(out.APIKeys, mktapi.KeyNeed{Name: k.Name, Label: k.Label, Description: k.Description})
			}
			o := &out.APIKeys[i]
			o.Required = o.Required || k.Required
			if o.Label == "" {
				o.Label = k.Label
			}
			if o.Description == "" {
				o.Description = k.Description
			}
			o.NeededBy = union(o.NeededBy, k.NeededBy)
		}
		for _, a := range r.Auth {
			if a.Name == "" {
				continue
			}
			i, seen := logins[a.Name]
			if !seen {
				i = len(out.Auth)
				logins[a.Name] = i
				out.Auth = append(out.Auth, mktapi.LoginNeed{Name: a.Name, Label: a.Label, Description: a.Description, LoginHint: a.LoginHint})
			}
			o := &out.Auth[i]
			if o.LoginHint == "" {
				o.LoginHint = a.LoginHint
			}
			if o.Label == "" {
				o.Label = a.Label
			}
			o.NeededBy = union(o.NeededBy, a.NeededBy)
		}
	}
	sort.SliceStable(out.Tools, func(i, j int) bool {
		return strings.ToLower(out.Tools[i].Name) < strings.ToLower(out.Tools[j].Name)
	})
	sort.SliceStable(out.APIKeys, func(i, j int) bool { return out.APIKeys[i].Name < out.APIKeys[j].Name })
	sort.SliceStable(out.Auth, func(i, j int) bool { return out.Auth[i].Name < out.Auth[j].Name })
	return out
}

func recipeKey(r publish.InstallRecipe) string {
	return r.Platform + "|" + r.Kind + "|" + r.Package + "|" + r.URL + "|" + r.Script
}

// union appends to a the values of b it does not hold yet, in order.
func union(a, b []string) []string {
	for _, v := range b {
		if v != "" && !contains(a, v) {
			a = append(a, v)
		}
	}
	return a
}
