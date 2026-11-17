// SPDX-License-Identifier: Apache-2.0

package core

// A FRESH VERDICT IS AN ANSWER. verify - a gate's
// question, `gate('ready-to-test')` - must answer within the gate's bound,
// but a criterion's check may take minutes (a live GUI test, a scenario
// case). So verify() counts a criterion from its recorded verdict when that
// verdict is fresh - recorded within FreshVerdict, for the probe the criterion
// declares now - or, for a live GUI walk, from its latest verdict whatever its
// age (walkVerdict), and runs the verify hooks of every other object of the
// subtree. Verify (verification verify) and object-run-hooks still run
// everything.

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	acceptanceapi "github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// FreshVerdict is how long a recorded verdict answers for its criterion.
const FreshVerdict = time.Hour

// freshVerdict is a criterion's verdict when it may answer for it: of the
// type verdict, under the criterion, recorded within FreshVerdict of now for
// the probe the criterion declares now.
func freshVerdict(criterion cwmapi.Object, children []cwmapi.Object, now time.Time) (cwmapi.HookRun, bool) {
	return recordedVerdict(criterion, children, func(ran time.Time) bool {
		return now.Sub(ran) <= FreshVerdict && !now.Before(ran.Add(-time.Minute))
	})
}

// A LIVE GUI WALK IS COUNTED, NEVER RUN, BY verify.
// A criterion whose probe is a GUI walk (api.GUIWalk) is run by the GUI batch
// alone, on its scenario case: verify() - a gate's question, run in no GUI
// environment - counts it from its latest verdict for the probe it declares
// now, whatever its age, and with none reads it unproven; its hook never runs,
// so no environment failure is written over the batch's verdict. An explicit
// Verify of the criterion still runs the walk.

// walkVerdict is verify()'s answer for a criterion whose probe is a GUI walk:
// its latest verdict for that probe, or unproven with none. false when the
// object is not such a criterion.
func walkVerdict(criterion cwmapi.Object, children []cwmapi.Object) (cwmapi.HookRun, bool) {
	check, ok := criterion.Capability(cwmapi.CapCheckable)
	if !ok {
		return cwmapi.HookRun{}, false
	}
	if probe, _ := check["probe"].(string); !api.GUIWalk(probe) {
		return cwmapi.HookRun{}, false
	}
	if run, ok := recordedVerdict(criterion, children, func(time.Time) bool { return true }); ok {
		run.Source = "gui batch verdict"
		return run, true
	}
	answer, _ := json.Marshal(map[string]string{"status": "unproven", "evidence": "a live GUI walk runs only in its GUI batch (scripts/gui-batch.sh): no verdict recorded for its probe"})
	return cwmapi.HookRun{ObjectID: criterion.ID, URI: "gui-batch", Source: "gui batch verdict", Answer: answer}, true
}

// recordedVerdict is the latest verdict under the criterion, recorded for the
// probe it declares now at a time keep accepts.
func recordedVerdict(criterion cwmapi.Object, children []cwmapi.Object, keep func(ran time.Time) bool) (cwmapi.HookRun, bool) {
	check, ok := criterion.Capability(cwmapi.CapCheckable)
	if !ok {
		return cwmapi.HookRun{}, false
	}
	probe, _ := check["probe"].(string)
	var best cwmapi.HookRun
	var bestAt time.Time
	found := false
	for _, v := range children {
		if v.Type != acceptanceapi.VerdictType {
			continue
		}
		grp, ok := v.Capability(acceptanceapi.CapVerdict)
		if !ok {
			continue
		}
		status, _ := grp["status"].(string)
		ranAt, _ := grp["ran_at"].(string)
		ran, err := time.Parse(time.RFC3339, ranAt)
		if status == "" || err != nil || !keep(ran) {
			continue
		}
		if was, _ := grp["probe"].(string); strings.TrimSpace(was) != strings.TrimSpace(probe) {
			continue
		}
		if found && !ran.After(bestAt) {
			continue
		}
		evidence, _ := grp["evidence"].(string)
		answer, _ := json.Marshal(map[string]string{"status": status, "verdict": v.ID, "evidence": evidence})
		best, bestAt, found = cwmapi.HookRun{ObjectID: criterion.ID, URI: "verdict:" + v.ID, Source: "fresh verdict", Answer: answer}, ran, true
	}
	return best, found
}

// verifyFresh is verify() over the object and everything under it: a
// criterion with a fresh verdict counted from it, every other object's verify
// hooks run.
func (s *Service) verifyFresh(ctx context.Context, cwm cwmapi.CWMService, id string) (api.VerifyOutput, error) {
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	return s.walkVerify(ctx, cwm, id, &now)
}

// walkVerify runs the verify point over the object and everything under it,
// one object at a time - depth first, children in id order, as a subtree run
// - and reports each check's answer as it comes (reportCheck). With fresh, a
// criterion with a verdict fresh at that time is counted from it instead of
// run.
//
// When the universe serves several projects the store set names the project
// holding each object (ObjectGetOutput.Project, ObjectListOutput.Projects)
// and answers its ids bare: the walk keeps that project and asks each
// object's children and runs its hooks by its qualified id <key>:<id>, so the
// subtree of another project's object never joins a homonym's children nor
// runs a homonym's hooks. One project served: no key
// the ids stay bare.
func (s *Service) walkVerify(ctx context.Context, cwm cwmapi.CWMService, id string, fresh *time.Time) (api.VerifyOutput, error) {
	root, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: moduleOf(id), ID: id})
	if err != nil {
		return api.VerifyOutput{}, err
	}
	rep := newCheckReporter(ctx, cwm, id)
	out := cwmapi.ObjectRunHooksOutput{Point: cwmapi.HookVerify, Runs: []cwmapi.HookRun{}}
	seen := map[string]bool{}
	var walk func(key string, o cwmapi.Object) error
	walk = func(key string, o cwmapi.Object) error {
		ref := o.ID
		if key != "" {
			ref = cwmapi.QualifiedID(key, o.ID)
		}
		if seen[ref] {
			return nil
		}
		seen[ref] = true
		kids, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: ref})
		if err != nil {
			return err
		}
		children := make([]located, len(kids.Objects))
		for i, k := range kids.Objects {
			children[i] = located{key: key, o: k}
			if i < len(kids.Projects) && kids.Projects[i] != "" {
				children[i].key = kids.Projects[i]
			}
		}
		sort.SliceStable(children, func(i, j int) bool { return children[i].o.ID < children[j].o.ID })
		objs := make([]cwmapi.Object, len(children))
		for i, c := range children {
			objs[i] = c.o
		}
		var runs []cwmapi.HookRun
		if run, ok := freshVerdictAt(o, objs, fresh); ok {
			out.Objects++
			runs = []cwmapi.HookRun{run}
		} else {
			r, err := cwm.ObjectRunHooks(ctx, cwmapi.ObjectRunHooksInput{Point: cwmapi.HookVerify, ID: ref})
			if err != nil {
				return err
			}
			out.Objects += r.Objects
			runs = r.Runs
		}
		out.Runs = append(out.Runs, runs...)
		rep.object(runs)
		for _, c := range children {
			if err := walk(c.key, c.o); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root.Project, root.Object); err != nil {
		return api.VerifyOutput{}, err
	}
	return readVerify(out), nil
}

// located is an object and the key of the project holding it ("" when one
// project is served).
type located struct {
	key string
	o   cwmapi.Object
}

// freshVerdictAt is verify()'s recorded answer at fresh - a GUI walk's
// latest verdict (walkVerdict), else a fresh one - none without a time (an
// explicit Verify runs everything).
func freshVerdictAt(o cwmapi.Object, children []cwmapi.Object, fresh *time.Time) (cwmapi.HookRun, bool) {
	if fresh == nil {
		return cwmapi.HookRun{}, false
	}
	if run, ok := walkVerdict(o, children); ok {
		return run, true
	}
	return freshVerdict(o, children, *fresh)
}

// moduleOf is the module part of <module>.<local>.
func moduleOf(id string) string {
	m, _, _ := strings.Cut(id, ".")
	return m
}
