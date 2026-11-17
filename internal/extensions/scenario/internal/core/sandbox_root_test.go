// SPDX-License-Identifier: Apache-2.0

package core

import (
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/sandbox"
)

// : a process marked inside a case's sandbox but on another root
// (an installation's program that took its installation's roots back) works
// on nothing of the case - the admin universe of the installation is never
// configured for the case.
func TestACaseIsWorkedOnlyOnItsSandboxRoot(t *testing.T) {
	caseDir := filepath.Join(t.TempDir(), "demo")
	e := &Engine{Case: &api.Case{Name: "demo"}, RT: Runtime{Dir: caseDir}}
	sb := sandbox.Of(caseDir)
	t.Setenv(sandbox.Marker, sb.Case)
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(t.TempDir(), "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	if err := e.RequireSandbox(); err == nil {
		t.Fatal("on an installation's root, the case was worked on")
	}
	if err := e.ensureMachine(); err == nil {
		t.Fatal("on an installation's root, the sandbox's admin universe was written")
	}
	t.Setenv("KTW_KATYOTI_ROOT", sb.Root())
	application.ResetGlobals()
	if err := e.RequireSandbox(); err != nil {
		t.Fatal(err)
	}
}
