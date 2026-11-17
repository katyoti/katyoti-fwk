// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
	"testing"
)

// KTW_GUI_DEBUG_PORT opens the debugging port it names; unset, nothing; a
// value that is not a port, nothing and an error to log.
func TestTheDebugPortOpensOnlyWhenAPortIsAsked(t *testing.T) {
	if args, err := debugPortArgs("9222"); err != nil || !reflect.DeepEqual(args, []string{"--remote-debugging-port=9222"}) {
		t.Fatalf("9222: %v %v", args, err)
	}
	if args, err := debugPortArgs(""); err != nil || args != nil {
		t.Fatalf("unset: %v %v", args, err)
	}
	for _, bad := range []string{"abc", "80", "70000", "9222 --no-sandbox"} {
		if args, err := debugPortArgs(bad); err == nil || args != nil {
			t.Errorf("%q: %v %v, want refused", bad, args, err)
		}
	}
}
