// SPDX-License-Identifier: Apache-2.0

package hubharness

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// isolateTestbin points the harness's cache and run folders at the test's own
// temporary folder, and gives the process's run folder back after.
func isolateTestbin(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, tmp)
	}
	runMu.Lock()
	savedDir, savedCopies := runDir, runCopies
	runDir, runCopies = "", map[string]string{}
	runMu.Unlock()
	t.Cleanup(func() {
		runMu.Lock()
		runDir, runCopies = savedDir, savedCopies
		runMu.Unlock()
	})
}

// assertRunnable checks a program's copy: the source's bytes and, where the
// system has exec bits, the owner's exec bit.
func assertRunnable(t *testing.T, p string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s does not hold the source's bytes", p)
	}
	if runtime.GOOS == "windows" {
		return // no exec bits: the content is the whole check
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("%s is not executable: mode %v", p, info.Mode().Perm())
	}
}

// A program's cached copy and its run copy keep the source's exec bits: the
// cache's file is made by CreateTemp (0600) and copyFile only opens it, so
// without the chmod the run copy (a hard link to it) cannot run on Linux.
// Falsified by keepMode doing nothing: both copies stay 0600 (Linux, macOS).
func TestAProgramsRunCopyKeepsItsExecBits(t *testing.T) {
	isolateTestbin(t)
	bin := t.TempDir()
	src := filepath.Join(bin, exeName("ktw-mode-probe"))
	body := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(src, body, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := cacheCopy(src, info)
	if err != nil {
		t.Fatalf("cacheCopy: %v", err)
	}
	assertRunnable(t, cached, body)

	run, err := runCopy(bin, "ktw-mode-probe")
	if err != nil || run == "" {
		t.Fatalf("runCopy: %q, %v", run, err)
	}
	assertRunnable(t, run, body)
}

// A cached copy an older harness left at 0600 (the stamp is size and mtime,
// so the same build keeps its name) is given its exec bits when it is used.
// Falsified by cacheCopy answering an existing copy as it is.
func TestAStaleCachedCopyIsMadeExecutable(t *testing.T) {
	isolateTestbin(t)
	bin := t.TempDir()
	src := filepath.Join(bin, exeName("ktw-mode-probe"))
	body := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(src, body, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := cacheCopy(src, info)
	if err != nil {
		t.Fatalf("cacheCopy: %v", err)
	}
	if err := os.Chmod(cached, 0o600); err != nil { // what an older harness left
		t.Fatal(err)
	}
	again, err := cacheCopy(src, info)
	if err != nil || again != cached {
		t.Fatalf("cacheCopy again: %q, %v (want %q)", again, err, cached)
	}
	assertRunnable(t, again, body)
}
