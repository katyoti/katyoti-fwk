// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// TRIGGERS — the declared filers, as data like the work templates: a change
// to an object of their types (goal-changed), or a rule it fails
// (goal-served). planification's hooks call Fire on every create and update of
// those types with the object and what it was; Fire tells a real change from
// planning's own writes — a request filed on the object, its plan, a to-do —
// which change nothing, so a trigger never fires on its own work.

//go:embed triggers/*.yaml
var triggerFS embed.FS

var (
	triggersOnce sync.Once
	triggerList  []api.Trigger
	triggersErr  error
)

// loadTriggers reads the shipped triggers once.
func loadTriggers() ([]api.Trigger, error) {
	triggersOnce.Do(func() {
		triggerList, triggersErr = readTriggers(triggerFS, "triggers")
	})
	return triggerList, triggersErr
}

// readTriggers reads and checks the triggers of a folder, by name.
func readTriggers(fsys fs.FS, dir string) ([]api.Trigger, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var out []api.Trigger
	for _, e := range entries {
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var tr api.Trigger
		if err := config.DecodeBytes(raw, marshal.YAML, &tr); err != nil {
			return nil, fmt.Errorf("trigger %s: %w", e.Name(), err)
		}
		if err := checkTrigger(tr); err != nil {
			return nil, fmt.Errorf("trigger %s: %w", e.Name(), err)
		}
		out = append(out, tr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// checkTrigger refuses a trigger that could not fire or could not file.
func checkTrigger(tr api.Trigger) error {
	switch {
	case tr.Name == "" || len(tr.Types) == 0:
		return fmt.Errorf("a trigger has a name and the types it watches")
	case tr.On != api.OnChange && tr.On != api.OnRule:
		return fmt.Errorf("on is change or rule, got %q", tr.On)
	case tr.On == api.OnRule && tr.Check == "":
		return fmt.Errorf("a rule has a check (an el expression that holds when the object is fine)")
	case !slices.Contains(api.Kinds, tr.File.Kind) || tr.File.Title == "":
		return fmt.Errorf("it files a request of a known kind with a title, got %+v", tr.File)
	}
	if tr.Grace != "" {
		if tr.On != api.OnRule {
			return fmt.Errorf("only a rule has a grace")
		}
		if d, err := time.ParseDuration(tr.Grace); err != nil || d <= 0 {
			return fmt.Errorf("grace is a positive duration (1m), got %q", tr.Grace)
		}
	}
	return nil
}

// graceOverride, when set, replaces every rule's grace (tests).
var graceOverride atomic.Int64

// SetTriggerGrace replaces every rule's grace by d (0: each its own); it
// answers the restore.
func SetTriggerGrace(d time.Duration) func() {
	prev := graceOverride.Swap(int64(d))
	return func() { graceOverride.Store(prev) }
}

// graceOf is how long a rule waits before it is checked.
func graceOf(tr api.Trigger) time.Duration {
	if tr.Grace == "" {
		return 0
	}
	if d := graceOverride.Load(); d > 0 {
		return time.Duration(d)
	}
	d, _ := time.ParseDuration(tr.Grace)
	return d
}

// List answers the declared triggers.
func (t *Triggers) List(context.Context, api.ListTriggersInput) (api.TriggersOutput, error) {
	triggers, err := loadTriggers()
	if err != nil {
		return api.TriggersOutput{}, err
	}
	return api.TriggersOutput{Triggers: triggers}, nil
}

// Fire raises the requests of the triggers an object's create or update fires.
func (t *Triggers) Fire(ctx context.Context, in cwmapi.HookCall) (api.FireOutput, error) {
	out := api.FireOutput{Fired: []api.RaiseOutput{}}
	if in.Object == nil || (in.Point != cwmapi.HookOnCreate && in.Point != cwmapi.HookOnUpdate) {
		return out, nil
	}
	triggers, err := loadTriggers()
	if err != nil {
		return out, err
	}
	obj := *in.Object
	changed, moved := "", ""
	if in.Prior != nil {
		changed = contentChange(*in.Prior, obj)
		if from, to := ownState(*in.Prior), ownState(obj); from != to {
			moved = "it moved from " + from + " to " + to
		}
	}
	var cwm cwmapi.CWMService
	for _, tr := range triggers {
		if !slices.Contains(tr.Types, obj.Type) || (len(tr.States) > 0 && !slices.Contains(tr.States, ownState(obj))) {
			continue
		}
		src := api.RequestSource{Name: tr.Name, Object: obj.ID}
		switch tr.On {
		case api.OnChange:
			if changed == "" {
				continue
			}
			if tr.Check != "" {
				// A change trigger's check says when the change matters (a use
				// case with something to run): it fires only when it holds.
				if cwm == nil {
					if cwm, err = cwmOf(ctx); err != nil {
						return out, err
					}
				}
				matters, err := ruleHolds(ctx, cwm, obj.ID, tr.Check)
				if err != nil {
					return out, fmt.Errorf("Fire: trigger %s on %s: %w", tr.Name, obj.ID, err)
				}
				if !matters {
					continue
				}
			}
			src.Kind, src.Change = api.SourceTrigger, changed
		case api.OnRule:
			// A rule is checked when the object appears, changes or moves —
			// never on planning's own writes.
			if in.Point == cwmapi.HookOnUpdate && changed == "" && moved == "" {
				continue
			}
			if g := graceOf(tr); g > 0 {
				// Its author is given the grace first: checked then, on the
				// object as it is by then.
				go t.later(context.WithoutCancel(ctx), tr, obj.ID, g)
				continue
			}
			if cwm == nil {
				if cwm, err = cwmOf(ctx); err != nil {
					return out, err
				}
			}
			fine, err := ruleHolds(ctx, cwm, obj.ID, tr.Check)
			if err != nil {
				return out, fmt.Errorf("Fire: rule %s on %s: %w", tr.Name, obj.ID, err)
			}
			if fine {
				continue
			}
			src.Kind, src.Change = api.SourceRule, "it fails: "+tr.Description
		}
		r, err := t.Raise(ctx, api.RaiseInput{Source: src, Kind: tr.File.Kind, Title: tr.File.Title, Details: tr.File.Details, On: tr.OnObject, Holds: tr.Holds, Priority: tr.File.Priority})
		if err != nil {
			return out, err
		}
		out.Fired = append(out.Fired, r)
	}
	return out, nil
}

// later checks a rule on an object once its grace has passed, and raises its
// request when the object, as it is then, fails it.
func (t *Triggers) later(ctx context.Context, tr api.Trigger, id string, grace time.Duration) {
	time.Sleep(grace)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return
	}
	obj, err := loadObject(ctx, cwm, id)
	if err != nil || (len(tr.States) > 0 && !slices.Contains(tr.States, ownState(obj))) {
		return
	}
	fine, err := ruleHolds(ctx, cwm, id, tr.Check)
	if err != nil || fine {
		return
	}
	src := api.RequestSource{Name: tr.Name, Object: id, Kind: api.SourceRule, Change: "it fails: " + tr.Description}
	_, _ = t.Raise(ctx, api.RaiseInput{Source: src, Kind: tr.File.Kind, Title: tr.File.Title, Details: tr.File.Details, On: tr.OnObject, Holds: tr.Holds, Priority: tr.File.Priority})
}

// ruleHolds evaluates a rule's check on an object.
func ruleHolds(ctx context.Context, cwm cwmapi.CWMService, id, check string) (bool, error) {
	got, err := cwm.ObjectEvaluate(ctx, cwmapi.ObjectEvaluateInput{ID: id, Expressions: []string{check}})
	if err != nil {
		return false, err
	}
	if len(got.Results) != 1 {
		return false, fmt.Errorf("no answer to %q", check)
	}
	r := got.Results[0]
	if r.Error != "" {
		return false, fmt.Errorf("%s: %s", check, r.Error)
	}
	ok, isBool := r.Value.(bool)
	if !isBool {
		return false, fmt.Errorf("%s answered %v, not true or false", check, r.Value)
	}
	return ok, nil
}

// processOwn is what the work's own process writes on an object — planning's
// capabilities, its lifecycle, its gate confirmations — and planning's links:
// none of it is a change to what the object says.
var (
	processOwn   = []string{api.CapRequest, api.CapPlan, api.CapGateWait, api.CapModuleUses, api.CapNetworkNode, "lifecycle", "gates"}
	processLinks = []string{api.EdgeDependsOn, api.EdgeResolves, api.EdgeSplitFrom, api.EdgeAskedIn, api.EdgeAbout, api.EdgeTargets, api.EdgeTakes,
		api.EdgeShipsIn, api.EdgeShips, api.EdgeReadsModule, api.EdgeWritesModule}
)

// contentChange says what changed in what an object says — its name, its
// description, its body, its fields, its own links, its other capabilities; ""
// when nothing did.
func contentChange(prior, now cwmapi.Object) string {
	var what []string
	if prior.Name != now.Name {
		what = append(what, "its name")
	}
	if prior.Description != now.Description {
		what = append(what, "its description")
	}
	if prior.Body != now.Body {
		what = append(what, "its body")
	}
	if !same(prior.Fields, now.Fields) {
		what = append(what, "its fields")
	}
	if !same(ownLinks(prior.Edges), ownLinks(now.Edges)) {
		what = append(what, "its links")
	}
	caps := map[string]bool{}
	for c := range prior.Capabilities {
		caps[c] = true
	}
	for c := range now.Capabilities {
		caps[c] = true
	}
	var changedCaps []string
	for c := range caps {
		if !slices.Contains(processOwn, c) && !same(judged(c, prior.Capabilities[c]), judged(c, now.Capabilities[c])) {
			changedCaps = append(changedCaps, c)
		}
	}
	sort.Strings(changedCaps)
	for _, c := range changedCaps {
		what = append(what, "its "+c)
	}
	if len(what) == 0 {
		return ""
	}
	return strings.Join(what, ", ") + " changed"
}

// judged is a capability's group as what judges the object sees it: a
// checkable's note says why or how it is measured — commentary, not what is
// measured — so a note alone changes nothing.
func judged(capability string, group map[string]any) map[string]any {
	if capability != "checkable" || group == nil {
		return group
	}
	out := make(map[string]any, len(group))
	for k, v := range group {
		if k != "note" {
			out[k] = v
		}
	}
	return out
}

// ownLinks is an object's links but planning's.
func ownLinks(edges []cwmapi.ObjectEdge) []cwmapi.ObjectEdge {
	var out []cwmapi.ObjectEdge
	for _, e := range edges {
		if !slices.Contains(processLinks, e.Kind) {
			out = append(out, e)
		}
	}
	return out
}

// same compares two values as their JSON (nil and empty alike).
func same(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	empty := func(j []byte) bool { s := string(j); return s == "null" || s == "{}" || s == "[]" }
	if empty(ja) && empty(jb) {
		return true
	}
	return reflect.DeepEqual(ja, jb)
}
