// SPDX-License-Identifier: Apache-2.0

package oslock

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// One holder at a time; Held sees it; the note is readable while held;
// unlocked, the next one takes it.
func TestOneHolderAtATime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x", "serving.lock")
	a, err := TryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TryLock(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("a second TryLock: %v; want ErrLocked", err)
	}
	if held, err := Held(path); err != nil || !held {
		t.Fatalf("Held = %v, %v; want held", held, err)
	}
	if err := a.Note("pid 42"); err != nil {
		t.Fatal(err)
	}
	if got := Note(path); got != "pid 42" {
		t.Errorf("note %q; want it readable while held", got)
	}
	if !a.StillHeld() {
		t.Error("StillHeld false on a held lock")
	}
	if err := a.Unlock(); err != nil {
		t.Fatal(err)
	}
	if held, _ := Held(path); held {
		t.Error("held after Unlock")
	}
	b, err := TryLock(path)
	if err != nil {
		t.Fatalf("after Unlock, TryLock: %v", err)
	}
	_ = b.Unlock()
	if held, err := Held(filepath.Join(t.TempDir(), "missing.lock")); held || err != nil {
		t.Errorf("a missing file: held=%v err=%v; want not held", held, err)
	}
}

// A process that dies releases its lock, however it dies (killed here).
func TestADeadHolderReleasesItsLock(t *testing.T) {
	if os.Getenv("OSLOCK_HOLD") != "" {
		l, err := TryLock(os.Getenv("OSLOCK_HOLD"))
		if err != nil {
			os.Exit(3)
		}
		_ = l.Note("child")
		os.Stdout.WriteString("locked\n")
		time.Sleep(time.Minute)
		return
	}
	path := filepath.Join(t.TempDir(), "held.lock")
	cmd := exec.Command(os.Args[0], "-test.run=TestADeadHolderReleasesItsLock")
	cmd.Env = append(os.Environ(), "OSLOCK_HOLD="+path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, _ := bufio.NewReader(out).ReadString('\n')
	if strings.TrimSpace(line) != "locked" {
		_ = cmd.Process.Kill()
		t.Fatalf("the child did not lock: %q", line)
	}
	if held, _ := Held(path); !held {
		t.Fatal("the child's lock is not seen")
	}
	if _, err := TryLock(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("TryLock while the child holds it: %v", err)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if held, _ := Held(path); !held {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the dead child's lock is still held")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A lock whose file was replaced no longer holds its path.
func TestALockKnowsWhenItsFileIsReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.lock")
	l, err := TryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Unlock()
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Skipf("the platform does not let a locked file be replaced (%v) — nothing to detect", err)
	}
	if l.StillHeld() {
		t.Fatal("StillHeld true after its file was replaced")
	}
}
