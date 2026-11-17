// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
	"testing"
)

// THE ORCHESTRATOR OPENS NO LINKED CHILD: project
// groups are retired, so the orchestrator publishes no method that clones or
// opens a group's linked child - a repository a group linked is attached as a
// member (attach). A method named after a linked child coming
// back fails here.
func TestTheOrchestratorOpensNoLinkedChild(t *testing.T) {
	for name := range OrchestratorServiceDef.Methods {
		if strings.Contains(strings.ToLower(name), "linked") {
			t.Errorf("the orchestrator publishes %s: a linked child is no longer opened - a repository a group linked is attached as a member", name)
		}
	}
}
