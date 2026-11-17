// SPDX-License-Identifier: Apache-2.0

package hubharness

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// A test process killed mid-run (a timeout, Ctrl-C, taskkill /F: no cleanup
// runs) takes the programs it started with it, and the ones they started.
// Falsified by tieSelf not joining the job: the programs outlive the kill.
func TestAKilledTestProcessTakesItsProgramsWithIt(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("nothing ties a process to its parent's death here")
	}
	testProc := exec.Command(os.Args[0], "-test.run=^$")
	testProc.Env = append(os.Environ(), "HUBHARNESS_PROCTREE=tied")
	if err := testProc.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testProc.Process.Kill(); _ = testProc.Wait() })

	// Its program and the program's own child.
	var programs []int
	for deadline := time.Now().Add(30 * time.Second); len(programs) < 2; {
		if time.Now().After(deadline) {
			t.Fatalf("the test process's programs never all started: %v", programs)
		}
		time.Sleep(50 * time.Millisecond)
		programs = descendants(testProc.Process.Pid)
	}
	t.Cleanup(func() { stopAll(programs) }) // a failed run leaves nothing behind
	if runtime.GOOS == "linux" {
		// The parent-death signal reaches the tied program only; the one it
		// started is its own to end (a hub's sidecars end with their stdio).
		programs = programs[:1]
	}

	// Killed outright: no cleanup of the test process runs.
	if err := testProc.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = testProc.Wait()
	if alive := waitGone(programs, 10*time.Second); len(alive) > 0 {
		t.Fatalf("programs outlive their killed test process: %v (of %v)", alive, programs)
	}
}

// While a test runs a program, the program's file in bin/ can be replaced:
// the test runs its own copy. Falsified by running the source itself — a
// running program's file cannot be written.
func TestARunningProgramLeavesItsBinFileReplaceable(t *testing.T) {
	bin := TempDir(t)
	src := filepath.Join(bin, exeName("ktw-harness-probe"))
	if err := copyFile(os.Args[0], src, 0o755); err != nil {
		t.Fatal(err)
	}
	run, err := runCopy(bin, "ktw-harness-probe")
	if err != nil || run == "" {
		t.Fatalf("run copy: %q, %v", run, err)
	}
	if filepath.Dir(run) == bin {
		t.Fatalf("the run copy %s is in bin/", run)
	}
	prog := exec.Command(run, "-test.run=^$")
	prog.Env = append(os.Environ(), "HUBHARNESS_PROCTREE=child")
	if err := Tie(prog); err != nil {
		t.Fatal(err)
	}
	if err := prog.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prog.Process.Kill(); _ = prog.Wait() })
	time.Sleep(300 * time.Millisecond) // running, its image mapped

	if err := os.WriteFile(src, []byte("rebuilt"), 0o755); err != nil {
		t.Fatalf("bin/'s file cannot be replaced while the test runs its program: %v", err)
	}
}
