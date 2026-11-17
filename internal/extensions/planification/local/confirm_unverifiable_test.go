// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A REALIZED FEATURE AN AGENT CANNOT VERIFY IS THE PERSON'S TO CONFIRM
// : two realized features, each with its criterion
// passed and its verification green. One draws a screen and no criterion runs
// its GUI test: the verify gate's gui-tested refuses an agent - its move to
// done falls to confirm, a person's - so « À faire » lists it as a confirm
// item saying why. The other an agent verifies: it is not the person's.
func TestARealizedFeatureAnAgentCannotVerifyIsThePersonsToConfirm(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	realized := func(title string, edges ...cwmapi.ObjectEdge) string {
		t.Helper()
		f := want(t, ctx, cwm, "feature", title, edges...)
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "built"}); err != nil {
			t.Fatal(err)
		}
		passVerdict(t, ctx, cwm, "billing", criterionFor(t, ctx, cwm, "billing", f, "go test ./internal/extensions/billing/..."))
		return f
	}
	screen := realized("The invoice pane shows its lines", cwmapi.ObjectEdge{Kind: "realizes", To: "app/ktw-gui/frontend/src/sections/InvoiceLines.tsx"})
	engine := realized("Invoices are totalled from their lines")
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{screen: "pass", engine: "pass"}})

	confirm := map[string]api.PersonItem{}
	for _, it := range personQueue(t, ctx, "").Items {
		if it.Confirm != nil {
			confirm[it.Object] = it
		}
	}
	it, ok := confirm[screen]
	if !ok {
		t.Fatalf("« À faire » does not ask the person to confirm %s, which no agent can verify (its GUI test not run): %+v", screen, confirm)
	}
	if it.Confirm.Accept != "confirm" || !strings.Contains(it.Asks, "cannot verify") {
		t.Errorf("the confirm item of %s: %+v; want Accept confirm, saying an agent cannot verify it", screen, it)
	}
	if _, ok := confirm[engine]; ok {
		t.Errorf("« À faire » asks the person to confirm %s, which an agent verifies: %s", engine, confirm[engine].Asks)
	}
}
