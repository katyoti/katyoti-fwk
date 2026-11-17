// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// : a start with --no-welcome, or KTW_NO_WELCOME set, opens no
// welcome screen; without either, nothing changes.
func TestAStartCanTurnTheWelcomeScreensOff(t *testing.T) {
	for _, c := range []struct {
		args []string
		env  string
		want bool
	}{
		{nil, "", false},
		{[]string{"--universe", "dev"}, "", false},
		{[]string{"--no-welcome"}, "", true},
		{[]string{"--universe=scn-study", "--no-welcome=true"}, "", true},
		{nil, "1", true},
		{nil, "true", true},
		{nil, "0", false},
		{nil, "false", false},
	} {
		if got := noWelcomeArg(c.args, c.env); got != c.want {
			t.Errorf("args %v, %s=%q: no welcome %v, want %v", c.args, NoWelcomeEnv, c.env, got, c.want)
		}
	}
}
