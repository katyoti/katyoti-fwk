// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func seqsOf(t *testing.T, path string) []int64 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e struct {
			Seq int64 `json:"seq"`
		}
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, e.Seq)
	}
	return out
}

// A session's journal is numbered once: a run restored after a restart goes on
// from the last seq written (it started again at 1), and two runs writing one
// session (a parent and its child) never write the same number.
func TestASessionsJournalIsNumberedOnce(t *testing.T) {
	dir := t.TempDir()
	first, err := NewSessionRecorder(dir, "t", "s-1")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		first.LLMJournal().Log("note", i)
	}
	_ = first.Close()

	// The process restarted: a new recorder on the same session.
	resetSessionCountersForTest()
	again, err := NewSessionRecorder(dir, "t", "s-1")
	if err != nil {
		t.Fatal(err)
	}
	child, err := NewSessionRecorder(dir, "t", "s-1")
	if err != nil {
		t.Fatal(err)
	}
	again.LLMJournal().Log("note", "a")
	child.LLMJournal().Log("note", "b")
	again.LLMJournal().Log("note", "c")
	_ = again.Close()
	_ = child.Close()

	got := seqsOf(t, filepath.Join(dir, "s-1", "llm.jsonl"))
	seen := map[int64]bool{}
	for i, s := range got {
		if seen[s] {
			t.Fatalf("seqs = %v; %d written twice", got, s)
		}
		seen[s] = true
		if i > 0 && s <= got[i-1] {
			t.Fatalf("seqs = %v; want them increasing", got)
		}
	}
	if len(got) != 6 || got[5] != 6 {
		t.Errorf("seqs = %v; want 1..6", got)
	}
}

func resetSessionCountersForTest() {
	sessionCounters.Lock()
	sessionCounters.byDir = map[string]*sessionCounter{}
	sessionCounters.Unlock()
}

// A journal that grows past its limit is rotated — the oldest dropped — so a
// run that lives for ever does not fill the disk; a session reopened after a
// rotation goes on numbering past what was rotated away.
func TestAJournalIsRotatedPastItsLimit(t *testing.T) {
	old := SetMaxJournalBytesForTest(300)
	t.Cleanup(func() { SetMaxJournalBytesForTest(old) })
	dir := t.TempDir()
	rec, err := NewSessionRecorder(dir, "t", "s-rot")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		rec.LLMJournal().Log("note", "some text to fill the journal")
	}
	_ = rec.Close()
	base := filepath.Join(dir, "s-rot")
	for _, f := range []string{"llm.jsonl", "llm.1.jsonl", "llm.2.jsonl"} {
		st, err := os.Stat(filepath.Join(base, f))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if st.Size() > 400 {
			t.Errorf("%s is %d bytes; want it rotated near the limit", f, st.Size())
		}
	}
	if _, err := os.Stat(filepath.Join(base, "llm.3.jsonl")); err == nil {
		t.Error("llm.3.jsonl kept; want only two rotated files")
	}

	resetSessionCountersForTest()
	again, err := NewSessionRecorder(dir, "t", "s-rot")
	if err != nil {
		t.Fatal(err)
	}
	again.LLMJournal().Log("note", "after")
	_ = again.Close()
	seqs := seqsOf(t, filepath.Join(base, "llm.jsonl"))
	if last := seqs[len(seqs)-1]; last != 61 {
		t.Errorf("seq after reopening = %d; want 61 (60 written before)", last)
	}
}
