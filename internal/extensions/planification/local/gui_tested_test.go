// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A FEATURE THAT CHANGES A SCREEN IS DONE ONLY WITH A GUI TEST:
// ready-to-test's gui-tested requirement refuses `verify` on a realized feature
// that realizes screen code until a criterion serving it runs a live GUI test
// (a tests/live/ script) and its verdict passed; a feature drawn on no screen
// is verified as before.
func TestAScreenFeatureIsNotDoneWithoutAGuiTest(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	realized := func(title string, edges ...cwmapi.ObjectEdge) string {
		t.Helper()
		f := want(t, ctx, cwm, "feature", title, edges...)
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "built"}); err != nil {
			t.Fatal(err)
		}
		return f
	}
	screen := realized("The invoice pane shows its lines", cwmapi.ObjectEdge{Kind: "realizes", To: "app/ktw-gui/frontend/src/sections/InvoicePane.tsx"})
	plain := realized("The invoices are mailed", cwmapi.ObjectEdge{Kind: "realizes", To: "internal/extensions/billing/mail.go"})
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{screen: "pass", plain: "pass"}})
	verify := func(id string) error {
		_, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: "done", Transition: "verify", By: session, Note: "its criteria pass"})
		return err
	}

	if err := verify(plain); err != nil {
		t.Fatalf("a feature drawn on no screen: %v", err)
	}
	if err := verify(screen); err == nil || !strings.Contains(err.Error(), "gui-tested") {
		t.Fatalf("a screen feature with no GUI test: %v; want it refused by gui-tested", err)
	}

	// Its criterion runs a live GUI test on a scenario case: refused while
	// its verdict has not passed, done once it has.
	crit := cwmapi.Object{Type: "acceptance-criterion", Parent: screen, Edges: []cwmapi.ObjectEdge{{Kind: api.EdgeServes, To: screen}}}
	crit.Name = "The invoice pane draws one row per line (GUI test by element ids)"
	_ = crit.SetCapability("checkable", map[string]any{"kind": "command", "probe": "bash app/ktw-gui/frontend/tests/live/invoice-pane.sh"})
	c, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: crit})
	if err != nil {
		t.Fatal(err)
	}
	if err := verify(screen); err == nil || !strings.Contains(err.Error(), "has not passed") {
		t.Fatalf("a screen feature whose GUI test never ran: %v; want it refused, the test not passed", err)
	}
	_, local, _ := strings.Cut(c.Object.ID, ".")
	v := cwmapi.Object{ID: "billing.verdict-" + local, Type: "verdict", Parent: c.Object.ID}
	v.Name = "Verdict"
	_ = v.SetCapability("verdict", map[string]any{"status": "pass", "kind": "command", "probe": "bash app/ktw-gui/frontend/tests/live/invoice-pane.sh"})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: v}); err != nil {
		t.Fatal(err)
	}
	if err := verify(screen); err != nil {
		t.Fatalf("a screen feature whose GUI test passed: %v", err)
	}
	if st := stateOf(t, ctx, cwm, screen); st != "done" {
		t.Errorf("verified, %s is %q; want done", screen, st)
	}
}

// A MODULE WHOSE CODE SHIPS OBJECT PANELS DRAWS A SCREEN (
// review): a feature of it with no realizes edge - planification's own Delivery
// view is such a feature - is refused without its GUI test too.
func TestAFeatureOfAModuleShippingPanelsNeedsItsGuiTest(t *testing.T) {
	t.Parallel()
	ctx, cwm := panelModule(t)
	id := panelFeature(t, ctx, cwm, "The invoices panel lists the late ones")
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{id: "pass"}})
	_, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: "done", Transition: "verify", By: session, Note: "try"})
	if err == nil || !strings.Contains(err.Error(), "gui-tested") {
		t.Fatalf("a feature of a module shipping panels, without a GUI test: %v; want it refused by gui-tested", err)
	}
}

// A FEATURE THAT CHANGES NO SCREEN IS NOT HELD BY THE GUI-TEST GATE
// : a feature of a module shipping panels that declares
// it changes none (no-screen: a code removal, a guard) is done without a live
// GUI criterion; the declaration does not excuse one that realizes screen code
// or whose criterion runs a live GUI test.
func TestANoScreenFeatureOfAPanelModuleIsDoneWithoutAGuiTest(t *testing.T) {
	t.Parallel()
	ctx, cwm := panelModule(t)
	removal := panelFeature(t, ctx, cwm, "The replaced invoice code is deleted", api.CapNoScreen)
	drawn := panelFeature(t, ctx, cwm, "The late invoices panel is deleted", api.CapNoScreen)
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: drawn, To: "app/ktw-gui/frontend/src/sections/LateInvoices.tsx", Kind: "realizes"}); err != nil {
		t.Fatal(err)
	}
	tested := panelFeature(t, ctx, cwm, "The invoices panel keeps its filters", api.CapNoScreen)
	criterionFor(t, ctx, cwm, "invoicing", tested, "bash app/ktw-gui/frontend/tests/live/invoice-filters.sh")
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{removal: "pass", drawn: "pass", tested: "pass"}})
	verify := func(id string) error {
		_, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: "done", Transition: "verify", By: session, Note: "its criteria pass"})
		return err
	}
	if err := verify(removal); err != nil {
		t.Fatalf("a no-screen feature of a panel module, without a GUI test: %v; want it done", err)
	}
	if err := verify(drawn); err == nil || !strings.Contains(err.Error(), "gui-tested") {
		t.Errorf("a no-screen feature realizing screen code: %v; want it refused by gui-tested", err)
	}
	if err := verify(tested); err == nil || !strings.Contains(err.Error(), "has not passed") {
		t.Errorf("a no-screen feature whose live GUI test never ran: %v; want it refused, the test not passed", err)
	}
}

// A DEPENDENCY WHOSE ONLY MISSING PROOF IS A BATCHED GUI TEST IS MET FOR WHAT
// FOLLOWS (RQ-324): A realized, its other criterion
// passed, its live GUI test a deliverable of B, the increment's GUI batch; C
// depends on A and B on C. C is built before any batch and after it (no
// deadlock); the verify round of A waits on the batch, is ready once B ran the
// test, and A ends done - verified without the person once its live verdict
// passes (RQ-444).
func TestABatchedGUIProofDoesNotHoldWhatFollows(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	script := "app/ktw-gui/frontend/tests/live/invoice-pane.sh"
	a := want(t, ctx, cwm, "feature", "The invoice pane shows its lines", cwmapi.ObjectEdge{Kind: "realizes", To: "app/ktw-gui/frontend/src/sections/InvoicePane.tsx"})
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: a, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: a, To: "realized", By: session, Note: "built"}); err != nil {
		t.Fatal(err)
	}
	unit := criterionFor(t, ctx, cwm, "billing", a, "go test ./internal/extensions/billing/...")
	passVerdict(t, ctx, cwm, "billing", unit)
	live := criterionFor(t, ctx, cwm, "billing", a, "bash "+script)

	// The verify round of A, filed as its build ended realized.
	if _, err := req.File(ctx, api.FileInput{On: a, Kind: api.KindReview, Increment: in.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, a, api.StepIntake).ID, session, schedapi.Result{Summary: "a review"})

	c := file(t, ctx, req, api.FileInput{Title: "Remind late payers from the pane", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	if _, err := req.Depend(ctx, api.DependInput{ID: c.ID, On: a}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, c.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a fix"})
	if st := stepOf(t, ctx, c.ID, "implement"); st.State != schedapi.StateReady {
		t.Fatalf("A is built, only its GUI test not run - no batch yet (ops.RQ-324): the fix of C %s (%q); want ready", st.State, st.Waits)
	}

	title := "The GUI test of the invoice pane by element ids"
	b := file(t, ctx, req, api.FileInput{Title: "Run the GUI tests of the increment together", Module: "billing", Kind: api.KindBuild, Increment: in.ID,
		Deliverables: []api.Deliverable{{Title: title, Check: "bash " + script}}})
	if _, err := req.Depend(ctx, api.DependInput{ID: b.ID, On: c.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Depend(ctx, api.DependInput{ID: c.ID, On: a}); err != nil { // settles C again
		t.Fatal(err)
	}
	if st := stepOf(t, ctx, c.ID, "implement"); st.State != schedapi.StateReady {
		t.Fatalf("the only missing proof of A is batched in B: the fix of C %s (%q); want ready", st.State, st.Waits)
	}
	// The verify round of A waits on the batch, not on a person.
	if st := stepOf(t, ctx, a, "verify"); st.State != schedapi.StateWaiting || !strings.Contains(st.Waits, b.ID) {
		t.Fatalf("the verify round of A before the batch ran: %s (%q); want waiting on %s", st.State, st.Waits, b.ID)
	}

	// B runs the GUI test: the round is ready, the live verdict passes, A ends done.
	if _, err := req.Verify(ctx, api.VerifyInput{ID: b.ID, Deliverable: title, Evidence: "the batch ran it: ok", By: person}); err != nil {
		t.Fatal(err)
	}
	// Its step reports what it ran: the collection settles every to-do.
	pullAndReport(t, ctx, stepOf(t, ctx, b.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "the GUI tests ran"})
	if st := stepOf(t, ctx, a, "verify"); st.State != schedapi.StateReady {
		t.Fatalf("the verify round of A once the batch ran: %s (%q); want ready", st.State, st.Waits)
	}
	// Its live verdict passes: every criterion of A passed, nothing tested by
	// hand - planification's observer verifies it without the person
	// . The test does not move it itself: the observer
	// runs on its own goroutine and would race that move.
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{a: "pass"}})
	passVerdict(t, ctx, cwm, "billing", live)
	eventually(t, "A done once its batched GUI test passed, verified without the person and naming "+live, func() (string, bool) {
		note := doneNote(t, ctx, a)
		return stateOf(t, ctx, cwm, a) + ": " + note, strings.Contains(note, "verified without the person") && strings.Contains(note, live)
	})
}

// ONLY A GUI WALK IS A GUI TEST:
// a tests/live probe that drives no GUI (a CLI probe on a scenario case) is no
// GUI test by element ids - a screen feature whose only live criterion is one,
// passed, is still refused by gui-tested, as having no GUI test; and a
// no-screen feature of a panel module with such a criterion is not held to a
// screen for it.
func TestACLILiveProbeIsNoGuiTest(t *testing.T) {
	t.Parallel()
	ctx, cwm := panelModule(t)
	cli := "bash tests/live/invoice-export.sh"
	screen := panelFeature(t, ctx, cwm, "The invoices panel exports its rows")
	passVerdict(t, ctx, cwm, "invoicing", criterionFor(t, ctx, cwm, "invoicing", screen, cli))
	// Its CLI probe has no verdict yet (its verify answers pass): held to a
	// screen, it would wait on that probe as its GUI test.
	quiet := panelFeature(t, ctx, cwm, "The invoices are exported nightly", api.CapNoScreen)
	criterionFor(t, ctx, cwm, "invoicing", quiet, cli)
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{screen: "pass", quiet: "pass"}})
	verify := func(id string) error {
		_, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: "done", Transition: "verify", By: session, Note: "its criteria pass"})
		return err
	}
	if err := verify(screen); err == nil || !strings.Contains(err.Error(), "no criterion serving it runs a GUI test") {
		t.Errorf("a screen feature whose only live criterion is a passed CLI probe: %v; want it refused by gui-tested, no GUI test", err)
	}
	if err := verify(quiet); err != nil && stateOf(t, ctx, cwm, quiet) != "done" {
		t.Errorf("a no-screen feature whose live criterion is a CLI probe: %v; want it done, held to no screen", err)
	}
}

// panelModule is a world whose module invoicing ships object panels.
func panelModule(t *testing.T) (context.Context, cwmapi.CWMService) {
	t.Helper()
	ctx, cwm, _ := worldWith(t, fstest.MapFS{"capabilities/code.yaml": {Data: []byte("name: code\n")}})
	pkg := "internal/extensions/invoicing"
	m := cwmapi.Object{ID: "invoicing", Type: "module"}
	m.Name = "invoicing"
	_ = m.SetCapability("code", map[string]any{"code_packages": []any{pkg}})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "invoicing", Object: m}); err != nil {
		t.Fatal(err)
	}
	// Its panels are shipped in the world's own project, never the workspace
	//
	root := projectRootOf(t, ctx, cwm, "invoicing")
	if repo := application.ProjectRoot(); repo != "" && filepath.Clean(root) == filepath.Clean(repo) {
		t.Fatalf("the world's project root %s is the process's own: the test would write the workspace", root)
	}
	panels := filepath.Join(root, filepath.FromSlash(pkg), "contrib", "embed", "cwm", "object-panels")
	if err := os.MkdirAll(panels, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(panels, "invoices.yaml"), []byte("name: invoices\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ctx, cwm
}

// panelFeature is a realized feature of invoicing, with its tags.
func panelFeature(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, title string, tags ...string) string {
	t.Helper()
	f := cwmapi.Object{Type: "feature", Parent: "invoicing"}
	f.Name = title
	_ = f.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
	for _, tag := range tags {
		_ = f.SetCapability(tag, map[string]any{})
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "invoicing", Object: f})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: out.Object.ID, To: "realized", By: session, Note: "built"}); err != nil {
		t.Fatal(err)
	}
	return out.Object.ID
}

// criterionFor is a criterion of the module serving the feature, run by probe.
func criterionFor(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, module, feature, probe string) string {
	t.Helper()
	crit := cwmapi.Object{Type: "acceptance-criterion", Parent: feature, Edges: []cwmapi.ObjectEdge{{Kind: api.EdgeServes, To: feature}}}
	crit.Name = "It holds: " + probe
	_ = crit.SetCapability("checkable", map[string]any{"kind": "command", "probe": probe})
	c, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: crit})
	if err != nil {
		t.Fatal(err)
	}
	return c.Object.ID
}

// passVerdict records the verdict of the criterion passed.
func passVerdict(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, module, criterion string) {
	t.Helper()
	_, local, _ := strings.Cut(criterion, ".")
	v := cwmapi.Object{ID: module + ".verdict-" + local, Type: "verdict", Parent: criterion}
	v.Name = "Verdict"
	_ = v.SetCapability("verdict", map[string]any{"status": "pass", "kind": "command"})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: v}); err != nil {
		t.Fatal(err)
	}
}
