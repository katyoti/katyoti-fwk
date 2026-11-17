// SPDX-License-Identifier: Apache-2.0

package hubharness

import (
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
)

// The helper modes: this test binary run again as a parent that starts a
// child, and as that child, both sleeping until killed.
func TestMain(m *testing.M) {
	switch os.Getenv("HUBHARNESS_PROCTREE") {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^$")
		child.Env = append(os.Environ(), "HUBHARNESS_PROCTREE=child")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	case "child":
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	case "tied": // a test process: its program, tied, starts one of its own
		prog := exec.Command(os.Args[0], "-test.run=^$")
		prog.Env = append(os.Environ(), "HUBHARNESS_PROCTREE=parent")
		if err := Tie(prog); err != nil {
			os.Exit(3)
		}
		if err := prog.Start(); err != nil {
			os.Exit(2)
		}
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// The harness lists a process's descendants while it lives, sees the ones that
// have not exited, and once it is killed ends them and waits for them.
func TestStopAllEndsWhatAKilledProcessLeft(t *testing.T) {
	parent := exec.Command(os.Args[0], "-test.run=^$")
	parent.Env = append(os.Environ(), "HUBHARNESS_PROCTREE=parent")
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Process.Kill(); _ = parent.Wait() })

	var members []int
	for deadline := time.Now().Add(30 * time.Second); len(members) == 0; {
		if time.Now().After(deadline) {
			t.Fatal("the parent's child never appeared among its descendants")
		}
		time.Sleep(50 * time.Millisecond)
		members = descendants(parent.Process.Pid)
	}
	if slices.Contains(members, parent.Process.Pid) {
		t.Fatalf("descendants names the root itself: %v", members)
	}

	t.Cleanup(func() { // a failed run leaves nothing behind
		for _, pid := range members {
			if p, err := os.FindProcess(pid); err == nil {
				_ = p.Kill()
			}
		}
	})
	// A process that has not exited is seen: the wait answers it.
	if alive := waitGone(members, 300*time.Millisecond); len(alive) != len(members) {
		t.Fatalf("waitGone missed a live process: alive %v of %v", alive, members)
	}
	_ = parent.Process.Kill()
	_ = parent.Wait()
	if alive := stopAll(members); len(alive) > 0 {
		t.Fatalf("stopAll left processes alive: %v", alive)
	}
}
