// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A RUN FILE DAMAGED INTO JSON THAT STILL PARSES IS SET ASIDE, NEVER RESUMED:
// its checksum no longer matches. A file written before the checksum is still
// read.
func TestACorruptedRunFileIsSetAsideNotResumed(t *testing.T) {
	dir := t.TempDir()
	st, err := newRunStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.close)
	var bad []string
	st.onBad = func(name string, err error) {
		if !errors.Is(err, errRunFileCorrupted) {
			t.Errorf("%s set aside for %v, not its checksum", name, err)
		}
		bad = append(bad, name)
	}

	// corrupt rewrites a run file in place, still valid JSON: the pid 4242
	// becomes 4243 (a bit the disk flipped).
	corrupt := func(path string) {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(b, []byte("4242")) {
			t.Fatalf("%s does not carry the value to damage: %s", path, b)
		}
		b = bytes.Replace(b, []byte("4242"), []byte("4243"), 1)
		if !json.Valid(b) {
			t.Fatal("the damaged file must still parse")
		}
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// A manifest: listed while whole, set aside once damaged.
	if err := st.saveManifest(runManifest{RunID: "r1", PID: 4242, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if ms, _ := st.manifests(); len(ms) != 1 || ms[0].PID != 4242 {
		t.Fatalf("a whole manifest is read back: %+v", ms)
	}
	corrupt(st.manifestPath("r1"))
	if ms, _ := st.manifests(); len(ms) != 0 {
		t.Fatalf("a damaged manifest was read as a run to restore: %+v", ms)
	}
	if len(bad) != 1 || bad[0] != "r1.run.json" {
		t.Fatalf("the damaged manifest was not reported: %v", bad)
	}
	if _, err := os.Stat(filepath.Join(dir, "bad", "r1.run.json")); err != nil {
		t.Fatalf("the damaged manifest was not set aside: %v", err)
	}

	// A checkpoint: loaded while whole, refused once damaged.
	cp := engine.Checkpoint{State: engine.State{Name: "waiting", RunID: "r2", Data: map[string]any{"n": 4242}},
		Wait: &engine.WaitSpec{Conditions: []engine.WaitCondition{{Topic: "wake.r2"}}, Mode: engine.WaitAny}}
	if err := st.Save("r2", cp); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load("r2"); err != nil {
		t.Fatalf("a whole checkpoint is loaded: %v", err)
	}
	corrupt(st.checkpointPath("r2"))
	if _, err := st.Load("r2"); !errors.Is(err, errRunFileCorrupted) {
		t.Fatalf("a damaged checkpoint was loaded (err %v): a run would resume from wrong state", err)
	}

	// A manifest written before the checksum is still read.
	if err := writeJSONAtomic(st.manifestPath("r3"), runManifest{RunID: "r3", PID: 7}); err != nil {
		t.Fatal(err)
	}
	if m, err := st.readManifest("r3"); err != nil || m.PID != 7 {
		t.Fatalf("a manifest written before the checksum is not read: %+v %v", m, err)
	}
}
