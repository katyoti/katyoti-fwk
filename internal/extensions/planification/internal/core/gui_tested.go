// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A FEATURE THAT CHANGES A SCREEN IS DONE ONLY WITH A GUI TEST (
// ): the ready-to-test requirement `gui-tested` asks it of a
// feature drawn on a screen - its module's code is the GUI or the display
// library, or it realizes a file there or a pack's object panel - and passes
// when one criterion serving it runs a live GUI test by element ids on a
// scenario case (a probe running a tests/live/ script, whose lib refuses any
// universe but the case's) and its last verdict passed. A feature drawn on no
// screen passes - and so does one of such a module that declares it changes
// none (the no-screen tag: a code removal, a guard), unless it realizes screen
// code or a criterion serving it runs a live GUI test.

// screenCode are the code paths that draw a screen: the GUI, and the display
// runtime and library its pages render with. A pack's object panels draw one
// too, wherever they are shipped (isScreenPath, shipsPanels).
var screenCode = []string{"app/ktw-gui", "app/ktw-display", "app/ktw-display-lib"}

// edgeRealizes is the edge from a feature to the code that realizes it.
const edgeRealizes = "realizes"

// liveGUITest marks a probe that runs a live test on a scenario case; whether
// it drives the GUI is verificationapi.GUIWalk's to say.
const liveGUITest = verificationapi.LiveTests

// GuiTested is the `gui-tested` requirement of ready-to-test, given the feature.
func (p *Planning) GuiTested(ctx context.Context, in api.HookInput) (api.CheckReadyOutput, error) {
	if in.Object == nil || in.Object.Type != featureType {
		return api.CheckReadyOutput{Accept: true}, nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.CheckReadyOutput{}, err
	}
	f := *in.Object
	if !drawsAScreen(ctx, cwm, f) {
		return api.CheckReadyOutput{Accept: true}, nil
	}
	live, _ := criteriaOf(ctx, cwm, f.ID)
	for _, c := range live {
		if verdictPassed(ctx, cwm, c.id) {
			return api.CheckReadyOutput{Accept: true}, nil
		}
	}
	if len(live) > 0 {
		return api.CheckReadyOutput{Reason: f.ID + " changes a screen: its live GUI test has not passed - run it on its scenario case (verification verify)"}, nil
	}
	return api.CheckReadyOutput{Reason: f.ID + " changes a screen: no criterion serving it runs a GUI test by element ids on a scenario case (a probe running app/ktw-gui/frontend/tests/live/<check>.sh)"}, nil
}

// drawsAScreen: one of its realizes edges points into screen code or at a
// pack's object panel, or its module draws a screen (screenModule) and it does
// not declare it changes none - a no-screen feature with a criterion running a
// live GUI test is held to it all the same.
func drawsAScreen(ctx context.Context, cwm cwmapi.CWMService, f cwmapi.Object) bool {
	for _, e := range f.Edges {
		if e.Kind == edgeRealizes && isScreenPath(e.To) {
			return true
		}
	}
	module, _, ok := strings.Cut(f.ID, ".")
	if !ok || !screenModule(ctx, cwm, module) {
		return false
	}
	if _, none := f.Capabilities[api.CapNoScreen]; none {
		live, _ := criteriaOf(ctx, cwm, f.ID)
		return len(live) > 0
	}
	return true
}

// criterion is a criterion serving a feature, with its probe.
type criterion struct{ id, probe string }

// criteriaOf splits the criteria serving a feature: those running a live GUI
// test on a scenario case, and the others.
func criteriaOf(ctx context.Context, cwm cwmapi.CWMService, feature string) (live, other []criterion) {
	ids, _ := incoming(ctx, cwm, feature, api.EdgeServes)
	for _, id := range ids {
		c, err := loadObject(ctx, cwm, id)
		if err != nil {
			continue
		}
		ck, has, _ := cwmapi.DecodeCapability[probeOf](c, capCheckable)
		if !has {
			continue
		}
		if verificationapi.GUIWalk(ck.Probe) {
			live = append(live, criterion{id, ck.Probe})
		} else {
			other = append(other, criterion{id, ck.Probe})
		}
	}
	return live, other
}

// liveScript is the tests/live/<check>.sh a live GUI probe runs; "" with none.
func liveScript(probe string) string {
	probe = strings.ReplaceAll(probe, "\\", "/")
	i := strings.Index(probe, liveGUITest)
	if i < 0 {
		return ""
	}
	rest := probe[i:]
	if j := strings.IndexAny(rest, " \t\"'"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// builtAwaitingGUI: a realized screen feature whose only missing proof is its
// live GUI test (rule) - every other
// criterion serving it (at least one) has passed, no live GUI test has. It is
// built for what depends on it, wherever its GUI test waits - its increment's
// batch, a release's, or none yet - and stays not done until that test passed.
func builtAwaitingGUI(ctx context.Context, cwm cwmapi.CWMService, f cwmapi.Object) bool {
	if f.Type != featureType || ownState(f) != featureRealized || !drawsAScreen(ctx, cwm, f) {
		return false
	}
	live, other := criteriaOf(ctx, cwm, f.ID)
	if len(other) == 0 {
		return false
	}
	for _, c := range other {
		if !verdictPassed(ctx, cwm, c.id) {
			return false
		}
	}
	for _, c := range live {
		if verdictPassed(ctx, cwm, c.id) {
			return false // gui-tested is met: nothing waits on a GUI test
		}
	}
	return true
}

// guiBatches is the GUI-test batches a built screen feature's proof waits in
// (RQ-324): each of its live GUI tests is a deliverable
// not yet verified of an open request - the requests running them together,
// an increment's or a release's. Nil when the feature is not built awaiting
// its GUI test, or one of its tests is in no batch.
func guiBatches(ctx context.Context, cwm cwmapi.CWMService, f cwmapi.Object) []string {
	if !builtAwaitingGUI(ctx, cwm, f) {
		return nil
	}
	live, _ := criteriaOf(ctx, cwm, f.ID)
	if len(live) == 0 {
		return nil
	}
	open := openBatchRequests(ctx, cwm)
	var batches []string
	for _, c := range live {
		b := batchRunning(open, liveScript(c.probe))
		if b == "" {
			return nil
		}
		if !slices.Contains(batches, b) {
			batches = append(batches, b)
		}
	}
	return batches
}

// openBatchRequests is every open request listing a live GUI test as a
// deliverable not yet verified, by id.
func openBatchRequests(ctx context.Context, cwm cwmapi.CWMService) []cwmapi.Object {
	all, _ := listRequests(ctx, cwm, "")
	var out []cwmapi.Object
	for _, r := range all {
		if planEnded(planState(r)) {
			continue
		}
		for _, d := range fieldsOf(r).Deliverables {
			if !d.Verified && liveScript(d.Check) != "" {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// batchRunning is the open request that lists the live script as a
// deliverable not yet verified; "" with none.
func batchRunning(open []cwmapi.Object, script string) string {
	if script == "" {
		return ""
	}
	for _, r := range open {
		for _, d := range fieldsOf(r).Deliverables {
			if !d.Verified && liveScript(d.Check) == script {
				return r.ID
			}
		}
	}
	return ""
}

func isScreenPath(p string) bool {
	p = strings.TrimPrefix(strings.ReplaceAll(p, "\\", "/"), "./")
	if strings.Contains(p, "/object-panels/") {
		return true
	}
	for _, s := range screenCode {
		if p == s || strings.HasPrefix(p, s+"/") {
			return true
		}
	}
	return false
}

// verdictPassed: the criterion's one verdict (<module>.verdict-<local>, the
// acceptance service's) says it passed.
func verdictPassed(ctx context.Context, cwm cwmapi.CWMService, criterion string) bool {
	return verdictPassedSince(ctx, cwm, criterion, "")
}

// verdictPassedSince: the criterion's verdict passed and was recorded at or
// after since (RFC 3339); an empty since, or a time either side does not
// carry, asks only that it passed.
func verdictPassedSince(ctx context.Context, cwm cwmapi.CWMService, criterion, since string) bool {
	return verdictStatusSince(ctx, cwm, criterion, since) == "pass"
}

// verdictStatusSince is the status of the criterion's recorded verdict when
// it ran at or after since (any time when since or its time is unknown); ""
// with none.
func verdictStatusSince(ctx context.Context, cwm cwmapi.CWMService, criterion, since string) string {
	module, local, ok := strings.Cut(criterion, ".")
	if !ok {
		return ""
	}
	v, err := loadObject(ctx, cwm, module+".verdict-"+local)
	if err != nil {
		return ""
	}
	st, _, _ := cwmapi.DecodeCapability[struct {
		Status string `json:"status"`
		RanAt  string `json:"ran_at"`
	}](v, "verdict")
	from, err1 := time.Parse(time.RFC3339, since)
	ran, err2 := time.Parse(time.RFC3339, st.RanAt)
	if err1 != nil || err2 != nil || !ran.Before(from) {
		return st.Status
	}
	return ""
}

// A DONE OBJECT WAITING ON ITS GUI BATCH: its to-dos
// are done and its lifecycle too, but its plan could not finish because
// deliverables that are live GUI walks are not verified yet - the walks run in
// a batch request (the release's final GUI batch) that lists them too. That is
// a wait on the batch, not an incident: the plan finishes once the batch has
// verified them.

// doneButNote begins the note of a plan blocked when its to-dos were done but
// it could not finish (collect).
const doneButNote = "its to-dos are done, but "

// liveScripts is every tests/live/<check>.sh a text names, once each - a
// hand test's steps may quote it (`…sh`) or end a sentence on it.
func liveScripts(text string) []string {
	rest := strings.ReplaceAll(text, "\\", "/")
	var out []string
	for {
		s := liveScript(rest)
		if s == "" {
			return out
		}
		rest = rest[strings.Index(rest, s)+len(s):]
		if i := strings.Index(s, ".sh"); i >= 0 {
			s = s[:i+len(".sh")]
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
}

// deliverableScripts is the live GUI walks a deliverable runs: its check's,
// or for a hand test, those its steps name.
func deliverableScripts(d api.Deliverable) []string {
	if s := liveScripts(d.Check); len(s) > 0 {
		return s
	}
	return liveScripts(d.ByHand)
}

// guiBatchRequests is every open request that can run a batch: its plan
// neither ended nor blocked, with a deliverable not yet verified that runs a
// live GUI walk.
func guiBatchRequests(ctx context.Context, cwm cwmapi.CWMService) []cwmapi.Object {
	all, _ := listRequests(ctx, cwm, "")
	var out []cwmapi.Object
	for _, r := range all {
		if ps := planState(r); planEnded(ps) || ps == api.PlanBlocked {
			continue
		}
		for _, d := range fieldsOf(r).Deliverables {
			if !d.Verified && len(deliverableScripts(d)) > 0 {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// guiBatchWait is the batches a done object's blocked plan waits on: every
// deliverable not yet verified (at least one) runs live GUI walks, each listed
// not yet verified by another of the batch requests. Nil otherwise - its plan
// is blocked for another reason, or a walk is in no batch.
func guiBatchWait(o cwmapi.Object, batches []cwmapi.Object) []string {
	pf, _ := planOf(o)
	if planState(o) != api.PlanBlocked || !strings.HasPrefix(pf.Note, doneButNote) {
		return nil
	}
	var out []string
	for _, d := range fieldsOf(o).Deliverables {
		if d.Verified {
			continue
		}
		scripts := deliverableScripts(d)
		if len(scripts) == 0 {
			return nil
		}
		for _, s := range scripts {
			b := batchListing(batches, o.ID, s)
			if b == "" {
				return nil
			}
			if !slices.Contains(out, b) {
				out = append(out, b)
			}
		}
	}
	return out
}

// batchListing is the batch request, other than self, listing the live walk
// as a deliverable not yet verified; "" with none.
func batchListing(batches []cwmapi.Object, self, script string) string {
	for _, r := range batches {
		if r.ID == self {
			continue
		}
		for _, d := range fieldsOf(r).Deliverables {
			if !d.Verified && slices.Contains(deliverableScripts(d), script) {
				return r.ID
			}
		}
	}
	return ""
}

// A VERIFY STEP WAITING ON ITS GUI BATCH: a verify
// reported blocked with no named kind, its reason naming the live GUI walks
// it could not run, each listed not yet verified by a batch request (the
// release's final GUI batch). That is a wait on the batch, not an
// unclassified one: the reconciler never retries it, and what waits after it
// waits on the batch too.

// walkName is a script a text names, with or without its folder.
var walkName = regexp.MustCompile(`[A-Za-z0-9_.-]+\.sh\b`)

// verifyBatchWait is the batches a blocked verify waits on: its report names
// no kind, and every script its reason names (at least one) is a live walk a
// batch request other than its own lists not yet verified. Nil otherwise.
func verifyBatchWait(t api.WorkTodo, batches []cwmapi.Object) []string {
	if t.Packet.Phase != api.PhaseVerify || stopStatus(t) != schedapi.StatusBlocked || (t.Held != nil && t.Held.Kind != "") {
		return nil
	}
	_, why := stopOf(t)
	if t.Held != nil && t.Held.Reason != "" {
		why = t.Held.Reason
	}
	names := walkName.FindAllString(strings.ReplaceAll(why, "\\", "/"), -1)
	if len(names) == 0 {
		return nil
	}
	var out []string
	for _, n := range names {
		b := batchListingNamed(batches, t.Request, n)
		if b == "" {
			return nil
		}
		if !slices.Contains(out, b) {
			out = append(out, b)
		}
	}
	return out
}

// batchListingNamed is the batch request, other than self, listing not yet
// verified a live walk of that file name; "" with none.
func batchListingNamed(batches []cwmapi.Object, self, name string) string {
	for _, r := range batches {
		if r.ID == self {
			continue
		}
		for _, d := range fieldsOf(r).Deliverables {
			if d.Verified {
				continue
			}
			for _, s := range deliverableScripts(d) {
				if path.Base(s) == name {
					return r.ID
				}
			}
		}
	}
	return ""
}
