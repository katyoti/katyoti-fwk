// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"strings"
	"testing"
)

// : ktw-agents publishes the pack kinds it
// carries once per binary. bootstrap.go names builtin.ContributeLeafKinds
// exactly once, after the per-layer skills/tasks loop - never inside it, where
// it would run once per global/universe layer.
func TestLeafKindsArePublishedOnceOutsideTheLayerLoop(t *testing.T) {
	src, err := os.ReadFile("bootstrap.go")
	if err != nil {
		t.Fatal(err)
	}
	code := strings.ReplaceAll(string(src), "\r\n", "\n")
	const call = "builtin.ContributeLeafKinds("
	if n := strings.Count(code, call); n != 1 {
		t.Fatalf("bootstrap.go names %s %d times, want exactly once", call, n)
	}
	loop := strings.Index(code, "range twoLayerLoaderDirs(")
	if loop < 0 {
		t.Fatal("bootstrap.go has no twoLayerLoaderDirs loop: update this guard")
	}
	// The loop's body ends at the first line closing a brace at one tab.
	end := strings.Index(code[loop:], "\n\t}\n")
	if end < 0 {
		t.Fatal("cannot find the end of the twoLayerLoaderDirs loop")
	}
	if at := strings.Index(code, call); at > loop && at < loop+end {
		t.Errorf("bootstrap.go calls %s inside the twoLayerLoaderDirs loop: it must run once per binary", call)
	}
}
