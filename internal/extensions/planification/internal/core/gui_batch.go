// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A BUILT FEATURE'S GUI TEST JOINS ITS RELEASE'S BATCH (
// rule): when gui-tested refuses a built screen feature
// (builtAwaitingGUI) whose live GUI tests are written, they are listed as
// deliverables of its release's GUI batch - the open request of the release
// whose checked deliverables are all live GUI tests - and that batch is filed
// once per release when none is open. A feature with no live test written,
// or no release, keeps the request asking its GUI test be written and run.

// guiTestedRequirement is the ready-to-test requirement a batch resolves.
const guiTestedRequirement = "gui-tested"

// batchGUITests lists the feature's live GUI tests in its release's batch;
// false when it cannot (not built, no live test, no release): the caller
// files the feature's own request.
func (s *Service) batchGUITests(ctx context.Context, cwm cwmapi.CWMService, f cwmapi.Object) (bool, error) {
	if !builtAwaitingGUI(ctx, cwm, f) {
		return false, nil
	}
	live, _ := criteriaOf(ctx, cwm, f.ID)
	if len(live) == 0 {
		return false, nil
	}
	inc, rel := releaseOfFeature(ctx, cwm, f)
	if rel == "" {
		return false, nil
	}
	var want []api.Deliverable
	for _, c := range live {
		if liveScript(c.probe) == "" {
			continue
		}
		title := f.ID + ": " + c.id
		if co, err := loadObject(ctx, cwm, c.id); err == nil && co.Name != "" {
			title = f.ID + ": " + co.Name
		}
		want = append(want, api.Deliverable{Title: title, Check: "bash " + liveScriptPath(c.probe)})
	}
	if len(want) == 0 {
		return false, nil
	}
	batch := releaseBatch(ctx, cwm, rel)
	if batch == nil {
		module, err := cwmapi.ModuleOfObjectID(rel)
		if err != nil {
			return false, err
		}
		name := rel
		if r, err := loadObject(ctx, cwm, rel); err == nil && r.Name != "" {
			name = r.Name
		}
		_, err = s.File(ctx, api.FileInput{
			Title:        "Run the GUI tests of " + name + " together",
			Details:      fmt.Sprintf("The release %s's GUI batch: the live GUI tests of its built screen features, run together on scenario cases at its end. A feature waiting only for its test here is built for what depends on it; it moves done once its test passed.", rel),
			Module:       module,
			Kind:         api.KindBuild,
			Increment:    inc,
			Deliverables: want,
			By:           planner,
		})
		return err == nil, err
	}
	have := fieldsOf(*batch).Deliverables
	next := append([]api.Deliverable(nil), have...)
	for _, d := range want {
		listed := false
		for _, h := range have {
			listed = listed || liveScript(h.Check) == liveScript(d.Check)
		}
		if !listed {
			next = append(next, d)
		}
	}
	if len(next) == len(have) {
		return true, nil
	}
	_, err := s.Revise(ctx, api.ReviseInput{ID: batch.ID, Deliverables: next, By: planner,
		Note: "The GUI test of " + f.ID + " joins the release's batch: the feature is built and waits only for it."})
	return err == nil, err
}

// releaseOfFeature is an increment the feature targets that ships in (or is
// planned for) a release, and that release; "" with none.
func releaseOfFeature(ctx context.Context, cwm cwmapi.CWMService, f cwmapi.Object) (inc, rel string) {
	if id := takerOf(ctx, cwm, f); id != "" {
		if r := releaseOfIncrement(ctx, cwm, id); r != "" {
			return id, r
		}
	}
	return "", ""
}

func releaseOfIncrement(ctx context.Context, cwm cwmapi.CWMService, id string) string {
	o, err := loadObject(ctx, cwm, id)
	if err != nil || o.Type != api.IncrementType {
		return ""
	}
	if r := edgeTo(o, api.EdgeShipsIn); r != "" {
		return r
	}
	return edgeTo(o, api.EdgePlannedFor)
}

// releaseBatch is the release's open GUI batch: an open request aimed at the
// release or one of its increments whose checked deliverables are all live
// GUI tests (at least one); nil with none.
func releaseBatch(ctx context.Context, cwm cwmapi.CWMService, rel string) *cwmapi.Object {
	for _, r := range openBatchRequests(ctx, cwm) {
		if !allLiveTests(fieldsOf(r).Deliverables) {
			continue
		}
		for _, kind := range []string{api.EdgeTargets, api.EdgeJoined} {
			for _, to := range edgesTo(r, kind) {
				if to == rel || releaseOfIncrement(ctx, cwm, to) == rel {
					r := r
					return &r
				}
			}
		}
	}
	return nil
}

// A LIVE WALK RUNS ONLY IN ITS BATCH: a deliverable
// whose check is a live GUI walk (verificationapi.GUIWalk) is never run by an agent on a
// request that is not a GUI batch - neither by requests check nor by the end
// gate. It is counted by the final GUI batch, which lists the walk (the batch
// request: every checked deliverable a GUI walk) and counts what the batch
// script recorded for it. Left unverified
// the request's end waits on the batch (guiBatchWait).
//
// A LIVE CHECK COUNTS BY ITS CRITERION'S VERDICT: a
// tests/live deliverable whose check is the probe of a criterion whose
// recorded verdict passed is verified from that verdict (verdictFor), never
// run again - on a GUI batch request, a verdict recorded since the request
// was filed; a tests/live script that drives no
// GUI is a CLI probe, run like any other check (its own lib keeps it
// read-only on a scenario case).

// countedByBatch is the answer for such a deliverable: not run, not passed.
const countedByBatch = "counted by the final GUI batch: a live walk runs only in its batch request"

// A GUI BATCH REQUEST ONLY COUNTS: its walks are run
// by scripts/gui-batch.sh alone - each on its scenario case, with the
// switches and timeouts the batch gives it, recording its criterion's
// verdict. Checking the batch request (requests check, its end gate) never
// runs a walk: it counts the passes recorded since it was filed (verdictFor)
// and leaves the other walks unverified (runByBatchScript).
const runByBatchScript = "not run here: run it with scripts/gui-batch.sh, which records its criterion's verdict"

// walkNotRun is why d, a GUI walk on a request (deliverables ds), is not run
// by a check: on a GUI batch it is the batch script's to run; on any other
// request it is counted by the batch. "" when d is not a GUI walk.
func walkNotRun(d api.Deliverable, ds []api.Deliverable) string {
	switch {
	case !verificationapi.GUIWalk(d.Check):
		return ""
	case allLiveTests(ds):
		return runByBatchScript
	default:
		return countedByBatch
	}
}

// liveScriptPath is the whole path of the tests/live script a probe runs -
// app/ktw-gui/frontend/tests/live/<walk>.sh kept whole, so the batch's check
// is still told a GUI walk (verificationapi.GUIWalk); "" with none.
func liveScriptPath(probe string) string {
	for _, w := range strings.Fields(strings.ReplaceAll(probe, "\\", "/")) {
		if strings.Contains(w, liveGUITest) {
			return strings.Trim(w, "\"'`")
		}
	}
	return ""
}

// allLiveTests says ds is a GUI batch: every checked deliverable a GUI walk.
func allLiveTests(ds []api.Deliverable) bool {
	n := 0
	for _, d := range ds {
		if strings.TrimSpace(d.Check) == "" {
			continue
		}
		if !verificationapi.GUIWalk(d.Check) {
			return false
		}
		n++
	}
	return n > 0
}

// verdictFor is the criterion whose probe d's check runs and whose recorded
// verdict passed, when d is a tests/live check on a request (deliverables ds,
// filed at filedAt); "" otherwise. On a GUI batch request the verdict counts
// only when recorded since the request was filed (the batch script records
// them): its walks with no such pass stay unverified
// until the script records one - a check never runs them (RQ-1153). On any
// request a pass older than the last commit touching the walk's script does
// not count (scriptChangedAt).
func verdictFor(ctx context.Context, cwm cwmapi.CWMService, d api.Deliverable, ds []api.Deliverable, filedAt string) string {
	if cwm == nil || liveScript(d.Check) == "" {
		return ""
	}
	since := ""
	if allLiveTests(ds) {
		since = filedAt
	}
	crits, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: capCheckable})
	if err != nil {
		return ""
	}
	key := probeKey(strings.TrimSpace(d.Check))
	for _, c := range crits.Objects {
		ck, has, _ := cwmapi.DecodeCapability[probeOf](c, capCheckable)
		if has && probeKey(strings.TrimSpace(ck.Probe)) == key &&
			verdictPassedSince(ctx, cwm, c.ID, laterOf(since, scriptChangedAt(ctx, cwm, c.ID, ck.Probe))) {
			return c.ID
		}
	}
	return ""
}

// failedVerdictFor is a criterion whose probe d's check runs and whose
// verdict recorded since since failed - a real failure of the batch script
// ; "" otherwise.
func failedVerdictFor(ctx context.Context, cwm cwmapi.CWMService, d api.Deliverable, since string) string {
	if cwm == nil || liveScript(d.Check) == "" {
		return ""
	}
	crits, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: capCheckable})
	if err != nil {
		return ""
	}
	key := probeKey(strings.TrimSpace(d.Check))
	for _, c := range crits.Objects {
		ck, has, _ := cwmapi.DecodeCapability[probeOf](c, capCheckable)
		if has && probeKey(strings.TrimSpace(ck.Probe)) == key && verdictStatusSince(ctx, cwm, c.ID, since) == "fail" {
			return c.ID
		}
	}
	return ""
}

// verifyByVerdict records ds[i] verified from criterion's passed verdict.
func verifyByVerdict(ds []api.Deliverable, i int, criterion string) {
	ds[i].Verified, ds[i].VerifiedAt = true, time.Now().UTC().Format(time.RFC3339)
	ds[i].Evidence = "the recorded verdict of " + criterion + " passed"
}

// A FAILED BATCHED GUI TEST FILES FIXES: when the
// batch script recorded a failed verdict for a walk of a release's GUI batch
// since the batch request was filed, the features whose live criterion runs that
// test get a fix request each, and so does every object depending on such a
// feature that was built or started building (builtOn) while the feature only
// waited for its test - one not started gets none. Filed once: an open
// request of the same title stands for it.
func (s *Service) fileGUIFailure(ctx context.Context, cwm cwmapi.CWMService, batch string, script, failure string) error {
	if script == "" {
		return nil
	}
	crits, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: capCheckable})
	if err != nil {
		return err
	}
	var errs []string
	for _, c := range crits.Objects {
		ck, has, _ := cwmapi.DecodeCapability[probeOf](c, capCheckable)
		if !has || liveScript(ck.Probe) != script {
			continue
		}
		for _, fid := range edgesTo(c, api.EdgeServes) {
			f, err := loadObject(ctx, cwm, fid)
			if err != nil || f.Type != featureType {
				continue
			}
			if err := s.fileFix(ctx, cwm, f, "Fix "+nameOf(f)+": its GUI test failed in "+batch,
				fmt.Sprintf("The GUI test %s of %s failed in the batch %s: %s", script, f.ID, batch, failure)); err != nil {
				errs = append(errs, err.Error())
			}
			deps, _ := incoming(ctx, cwm, f.ID, api.EdgeDependsOn)
			for _, did := range deps {
				d, err := loadObject(ctx, cwm, did)
				if err != nil || stateOf(d) == api.StateDropped || !builtOn(d) {
					continue
				}
				if err := s.fileFix(ctx, cwm, d, "Check "+nameOf(d)+" again: "+f.ID+"'s GUI test failed",
					fmt.Sprintf("%s was built while %s waited only for its GUI test; that test (%s) failed in the batch %s: check it still holds once %s is fixed.", d.ID, f.ID, script, batch, f.ID)); err != nil {
					errs = append(errs, err.Error())
				}
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// builtOn reports a dependent built on the feature, or started building on
// it, while the feature waited for its GUI test: a feature active, realized or
// done; a request done, or one a build step (or a later one) was recorded on.
// The failure is filed once, so a dependent still being built is checked now
// . One not started yet builds on the feature once it
// is fixed, and needs no check again.
func builtOn(d cwmapi.Object) bool {
	switch lifeState(d) {
	case featureActive, featureRealized, api.StateDone:
		return true
	}
	p, ok := planOf(d)
	if !ok {
		return false
	}
	build := api.PhaseIndex(api.PhaseBuild)
	for _, st := range p.Steps {
		if api.PhaseIndex(st.Phase) >= build {
			return true
		}
	}
	return false
}

// fileFix files a fix about the object in its module, unless an open request
// of that title is there already.
func (s *Service) fileFix(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, title, details string) error {
	module, err := cwmapi.ModuleOfObjectID(o.ID)
	if err != nil {
		return err
	}
	open, _ := listRequests(ctx, cwm, module)
	for _, r := range open {
		if r.Name == title && !planEnded(planState(r)) {
			return nil
		}
	}
	_, err = s.File(ctx, api.FileInput{Title: title, Details: details, Module: module, About: o.ID, Kind: api.KindFix, By: planner})
	return err
}
