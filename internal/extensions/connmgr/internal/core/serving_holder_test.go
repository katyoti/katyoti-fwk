// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// A MEMBER THAT CANNOT SERVE NAMES WHO DOES: a readiness error
// says which process holds the member's serving lock — a hub running
// unregistered beside this one held ktw-agents' for an hour, and every start
// only said "readiness deadline elapsed".
func TestTheReadinessErrorNamesTheProcessHoldingTheServingLock(t *testing.T) {
	base := errors.New("readiness deadline elapsed (last err: no responders)")
	spawned := &exec.Cmd{Process: &os.Process{Pid: 4242}}
	holder := "pid 777 (ktw-agents.exe), started by pid 700 (ktw-hub.exe)"
	scope := BusScope{ServingHolder: func(member string) string {
		if member == "ktw-agents" {
			return holder
		}
		return ""
	}}

	err := withServingHolder(base, scope, "ktw-agents", spawned)
	if !errors.Is(err, base) || !strings.Contains(err.Error(), holder) {
		t.Fatalf("the error does not name the holder: %v", err)
	}
	// Nobody holds it, or the member just spawned does (only slow): unchanged.
	if err := withServingHolder(base, scope, "ktw-other", spawned); err != base {
		t.Errorf("no holder: %v", err)
	}
	self := BusScope{ServingHolder: func(string) string { return "pid 4242 (ktw-agents.exe)" }}
	if err := withServingHolder(base, self, "ktw-agents", spawned); err != base {
		t.Errorf("the spawned member itself named as the holder: %v", err)
	}
	if err := withServingHolder(base, BusScope{}, "ktw-agents", spawned); err != base {
		t.Errorf("no ServingHolder: %v", err)
	}
	if holderIsPid("pid 42424 (x)", 4242) {
		t.Error("pid 42424 taken for pid 4242")
	}
}
