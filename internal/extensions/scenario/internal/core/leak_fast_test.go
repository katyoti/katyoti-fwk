// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// A PROCESS'S PATH IS TOLD WITHOUT STARTING A PROCESS: this
// test's own process answers its executable; a pid that runs nothing answers
// "". Falsified: processPath answering "" fails it.
func TestProcessPathTellsARunningProcess(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	got := processPath(os.Getpid())
	if !strings.EqualFold(filepath.Clean(got), filepath.Clean(self)) {
		resolved, _ := filepath.EvalSymlinks(self)
		if !strings.EqualFold(filepath.Clean(got), filepath.Clean(resolved)) {
			t.Fatalf("processPath(self) = %q, want %q", got, self)
		}
	}
	if p := processPath(0); p != "" {
		t.Errorf("processPath(0) = %q, want empty", p)
	}
}

// THE LEAK CHECK STAYS CHEAP AND KEEPS ITS MEANING: a cache
// file with the same size and time keeps the hash recorded before (it is not
// read again); one rewritten with other content is a leak, with the same
// content none; and no hub lock's holder is asked when nothing changed that
// needs it - each holder at most once when something does. Falsified: hashing
// every file again (the reused hash ignored) fails the first check; asking
// every lock's holder up front fails the two after it; telling a holder
// as often as it is judged fails the last.
func TestTheLeakCheckStaysCheapAndKeepsItsMeaning(t *testing.T) {
	base := t.TempDir()
	host := application.MachineDirs{Root: filepath.Join(base, "root"), Data: filepath.Join(base, "data"), Cache: filepath.Join(base, "cache")}
	write := func(rel, s string) string {
		p := filepath.Join(base, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	setTime := func(p string, at time.Time) {
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	write("root/universes.yaml", "universes: []\n")
	f := write("cache/katyoti/web/a.json", `{"a":1}`)
	setTime(f, stamp)
	for _, u := range []string{"dev", "other"} {
		if err := hubclient.WriteLockFile(filepath.Join(host.Data, "universes", u), hubclient.LockFile{PID: 1, NATSURL: "nats://127.0.0.1:1"}); err != nil {
			t.Fatal(err)
		}
	}

	// The same size and time: the hash recorded before is kept, not read.
	before := takeLeakSnapshot(host, nil)
	m := before[f]
	if m.Sum == "" {
		t.Fatalf("a small cache file has no hash: %+v", m)
	}
	m.Sum = "recorded-before"
	before[f] = m
	if got := takeLeakSnapshot(host, before)[f].Sum; got != "recorded-before" {
		t.Errorf("an unchanged cache file was hashed again: %q", got)
	}

	asked := map[string]int{}
	owner := leakOwner{
		lock: func(dir string) bool { asked[dir]++; return false },
		exe:  func(string) bool { return false },
	}
	check := func(act func()) (leaks, notes []string) {
		before := takeLeakSnapshot(host, nil)
		act()
		return before.leaks(takeLeakSnapshot(host, before), owner)
	}

	// The same content written again (a new time): no leak, no holder asked.
	leaks, _ := check(func() { write("cache/katyoti/web/a.json", `{"a":1}`); setTime(f, stamp.Add(time.Hour)) })
	if len(leaks) != 0 {
		t.Errorf("the same content rewritten: leaks %v", leaks)
	}
	if len(asked) != 0 {
		t.Errorf("nothing needed a lock's holder, yet asked %v", asked)
	}

	// Other content: a leak, still no holder asked (a plain cache file).
	leaks, _ = check(func() { write("cache/katyoti/web/a.json", `{"a":2}`); setTime(f, stamp.Add(2*time.Hour)) })
	if len(leaks) != 1 || !strings.Contains(leaks[0], "a.json") {
		t.Errorf("other content: leaks %v, want a.json", leaks)
	}
	if len(asked) != 0 {
		t.Errorf("a plain cache change asked a lock's holder: %v", asked)
	}

	// A person's sidecar record and their hub's lock changed: whether a
	// person's hub runs is asked, and who holds the lock - each holder once.
	rec := "cache/katyoti/sidecars/ktw-p@abc"
	write(rec+"/metadata.yaml", "exe_path: "+filepath.ToSlash(filepath.Join(base, "person", "ktw-p.exe"))+"\n")
	write(rec+"/manifest.json", `{"v":1}`)
	g := &LeakGuard{host: host, before: takeLeakSnapshot(host, nil)}
	write(rec+"/manifest.json", `{"v":2}`)
	if err := hubclient.WriteLockFile(filepath.Join(host.Data, "universes", "dev"), hubclient.LockFile{PID: 2, NATSURL: "nats://127.0.0.1:2"}); err != nil {
		t.Fatal(err)
	}
	leaks, notes := g.before.leaks(takeLeakSnapshot(host, g.before), owner)
	if len(leaks) != 0 || len(notes) == 0 {
		t.Errorf("a person's record and hub lock changed with their hub up: leaks %v notes %v; want notes only", leaks, notes)
	}
	for dir, n := range asked {
		if n > 1 {
			t.Errorf("the holder of %s asked %d times", dir, n)
		}
	}
}
