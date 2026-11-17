// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// inboxPackDir is the inbox extension's CWM pack, which declares both types.
const inboxPackDir = "../../../inbox/contrib/embed/cwm"

// TestInboxAllowsApprovalRequest guards the wiring that lets the agent's inbox
// channel file its questions: it creates `approval-request` objects under the
// inbox, which the containment rule refuses unless the two types admit each
// other. Loads the shipped pack so the inbox flow cannot silently break on a
// type edit.
func TestInboxAllowsApprovalRequest(t *testing.T) {
	if _, err := os.Stat(inboxPackDir); err != nil {
		t.Skipf("inbox pack not reachable from the test's directory (%v)", err)
	}
	types, err := LoadObjectTypesLayered(os.DirFS(inboxPackDir))
	if err != nil {
		t.Fatalf("LoadObjectTypesLayered: %v", err)
	}
	var inboxOK, requestOK bool
	for i := range types {
		switch types[i].Name {
		case "inbox":
			inboxOK = types[i].AllowsChild("approval-request")
		case "approval-request":
			requestOK = types[i].IsParentAllowed(api.ObjectType{Name: "inbox"})
		}
	}
	if !inboxOK || !requestOK {
		t.Errorf("the inbox must admit an approval-request (inbox side %v, request side %v)", inboxOK, requestOK)
	}
}
