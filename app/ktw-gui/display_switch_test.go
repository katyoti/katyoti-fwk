// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// An approved increment's switch is retired: the
// screens of IN-030, IN-038, IN-039, IN-047, IN-062, IN-063 and IN-064 show for everyone, so no
// variable is read for them and the GUI answers none of their switches.
func TestApprovedIncrementsSwitchesAreRetired(t *testing.T) {
	on := displaySwitches(func(string) string { return "1" })
	for _, name := range []string{"in035", "in030", "in038", "in039", "in047", "in062", "in063", "in064"} {
		if _, ok := switchEnvs[name]; ok {
			t.Errorf("%s's switch is still read: the person approved its delivery", name)
		}
		if _, ok := on[name]; ok {
			t.Errorf("the GUI still answers %s's switch", name)
		}
	}
	// IN-086's switch and IN-056's are retired at the
	// person's decision: the project mark and the increment's
	// Blockers tab show for everyone, their variables are never read. IN-061's
	// is retired with the never-shipped Sources cards tab it gated. IN-045's is retired with the
	// increment's close: the source model shows for everyone.
	// IN-1157's is retired with its delivery: the object card
	// children as cards and « Retirer » show for everyone. IN-035's
	// is retired with its delivery: the contribution
	// system review's screens show for everyone.
	for _, inc := range []string{"p.IN-086", "p.IN-056", "p.IN-061", "p.IN-045", "p.IN-1157", "p.IN-035"} {
		v := switchVariable(inc)
		for name, env := range switchEnvs {
			if env == v {
				t.Errorf("%s's switch %s is still read as %q: its screens show for everyone", inc, v, name)
			}
		}
	}
}

// TestDisplaySwitchVariableIsDerivedFromTheIncrement: the debug view's table
// names each switch's variable from its increment's id, so the
// variables the GUI reads must follow that one rule, and the states answer
// on only when the variable says so.
func TestDisplaySwitchVariableIsDerivedFromTheIncrement(t *testing.T) {
	for inc, want := range map[string]string{
		"p.IN-045": "KTW_IN045_DISPLAY",
		"IN-1007":  "KTW_IN1007_DISPLAY",
		"p.RL-034": "",
		"m.F-001":  "",
	} {
		if got := switchVariable(inc); got != want {
			t.Errorf("switchVariable(%q) = %q; want %q", inc, got, want)
		}
	}
	env := map[string]string{"KTW_IN045_DISPLAY": "yes", "KTW_IN099_DISPLAY": "0"}
	got := displaySwitchStates([]string{"p.IN-045", "p.IN-099", "x.RL-001"}, func(k string) string { return env[k] })
	want := []DisplaySwitchState{
		{Increment: "p.IN-045", Variable: "KTW_IN045_DISPLAY", On: true},
		{Increment: "p.IN-099", Variable: "KTW_IN099_DISPLAY", On: false},
		{Increment: "x.RL-001", Variable: "", On: false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("state %d = %+v; want %+v", i, got[i], want[i])
		}
	}
}
