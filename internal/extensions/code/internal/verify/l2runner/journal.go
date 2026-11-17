// SPDX-License-Identifier: Apache-2.0

package l2runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// JournalEvent is one parsed line from llm.jsonl or user.jsonl, normalised
// across both files so assertion code can treat them as a single time-ordered
// stream.
//
// The `File` field identifies the source ("llm" or "user") — useful when an
// assertion needs to disambiguate (e.g. UC-080 `prompt_shown` only lives in
// the user journal).
//
// `Data` is decoded as a raw `map[string]any` for flexible matching: each
// event payload shape is event-type-specific (per recorder.go in the task
// module), so a typed model would be brittle. Assertion code walks paths
// like `permissions.allowed_tools` over this map.
type JournalEvent struct {
	File      string
	Type      string
	Seq       int64
	Turn      int
	Timestamp time.Time
	Data      map[string]any
}

// captureJournal locates the freshly-written session dir produced by the
// invoke and returns its journal events, merged across both jsonl files and
// sorted by sequence number.
//
// Lookup strategy (in order):
//  1. `tmpHome/sessions/<latest>/` — the canonical path when the scenario
//     sets `KTW_DATA: $TMP_HOME` in its env block.
//  2. Recursive scan for any `sessions/<x>/llm.jsonl` under `tmpHome` —
//     covers scenarios that point `KTW_WORKSPACE` (or `KTW_DATA`) somewhere
//     unexpected.
//
// Returns (nil, nil) when no session dir exists — not all scenarios produce
// one (e.g. the C.1 canary `ktw-agents help`). Returns an error only on
// genuine read/parse failures of a session that DOES exist.
func captureJournal(tmpHome string) ([]JournalEvent, error) {
	sessionDir, err := findSessionDir(tmpHome)
	if err != nil {
		return nil, err
	}
	if sessionDir == "" {
		return nil, nil
	}

	var events []JournalEvent

	for _, kind := range []struct {
		file, label string
	}{
		{"llm.jsonl", "llm"},
		{"user.jsonl", "user"},
	} {
		entries, err := readJournalFile(filepath.Join(sessionDir, kind.file), kind.label)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", kind.file, err)
		}
		events = append(events, entries...)
	}

	sort.Slice(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
	return events, nil
}

// findSessionDir returns the absolute path of the most-recent session dir
// under tmpHome, or "" if none exists. A session dir is any directory under
// `<root>/sessions/` (regardless of nesting depth) that contains llm.jsonl
// OR user.jsonl.
func findSessionDir(tmpHome string) (string, error) {
	// Fast path: tmpHome/sessions/<single-child>.
	canonical := filepath.Join(tmpHome, "sessions")
	if dir := pickLatestChildWithJournal(canonical); dir != "" {
		return dir, nil
	}

	// Slow path: walk tmpHome looking for any */sessions/*/llm.jsonl.
	var candidates []string
	err := filepath.Walk(tmpHome, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // tolerate transient errors
		}
		if info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base != "llm.jsonl" && base != "user.jsonl" {
			return nil
		}
		parent := filepath.Dir(path)
		// Ensure the parent's parent is named "sessions" — guards against
		// false positives if a fixture happens to ship a stray llm.jsonl.
		if filepath.Base(filepath.Dir(parent)) != "sessions" {
			return nil
		}
		candidates = append(candidates, parent)
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", nil
	}
	// Deduplicate + pick the most recently modified.
	seen := map[string]bool{}
	uniq := candidates[:0]
	for _, c := range candidates {
		if !seen[c] {
			seen[c] = true
			uniq = append(uniq, c)
		}
	}
	return pickLatest(uniq), nil
}

// pickLatestChildWithJournal returns the most-recent direct subdir of `root`
// that contains a journal file, or "" when nothing qualifies.
func pickLatestChildWithJournal(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if hasJournalFile(dir) {
			paths = append(paths, dir)
		}
	}
	return pickLatest(paths)
}

func hasJournalFile(dir string) bool {
	for _, name := range []string{"llm.jsonl", "user.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func pickLatest(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	latest := paths[0]
	latestMod, _ := dirModTime(latest)
	for _, p := range paths[1:] {
		if t, err := dirModTime(p); err == nil && t.After(latestMod) {
			latest = p
			latestMod = t
		}
	}
	return latest
}

func dirModTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// readJournalFile parses one .jsonl file into events tagged with `fileLabel`.
// Tolerant of malformed lines: skips them with a soft warning rather than
// failing the whole capture.
func readJournalFile(path, fileLabel string) ([]JournalEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // one of the two files may legitimately be absent
		}
		return nil, err
	}
	defer f.Close()

	var events []JournalEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		raw, err := decodeJournalLine(line)
		if err != nil {
			continue // skip malformed lines — fail-soft per assertion authoring guide
		}
		raw.File = fileLabel
		events = append(events, raw)
	}
	return events, scanner.Err()
}

// decodeJournalLine parses one raw JSONL line into a JournalEvent. The
// recorder's `JournalEntry` / `UserJournalEntry` shapes share `type`, `seq`,
// `turn`, `ts`, `data`. We decode loosely into a transit struct then coerce.
func decodeJournalLine(line string) (JournalEvent, error) {
	var t struct {
		Type      string          `json:"type"`
		Seq       int64           `json:"seq"`
		Turn      int             `json:"turn"`
		Timestamp time.Time       `json:"ts"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(line), &t); err != nil {
		return JournalEvent{}, err
	}
	evt := JournalEvent{
		Type:      t.Type,
		Seq:       t.Seq,
		Turn:      t.Turn,
		Timestamp: t.Timestamp,
	}
	if len(t.Data) > 0 {
		_ = json.Unmarshal(t.Data, &evt.Data)
	}
	return evt, nil
}
