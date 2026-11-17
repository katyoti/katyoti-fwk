// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// Journal persists terminal messages as NDJSON (one event per line).
// Same format as the existing session recorder — replayable on connect.
type Journal struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	writer *bufio.Writer
	seqNo  int
}

// NewJournal creates or opens a terminal journal file.
func NewJournal(dir, sessionID string) (*Journal, error) {
	path := filepath.Join(dir, sessionID, "terminal.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("terminal journal: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("terminal journal: open: %w", err)
	}
	return &Journal{path: path, file: f, writer: bufio.NewWriter(f)}, nil
}

// Append writes an event to the journal. Returns the assigned seqNo and
// the bytes its line takes.
func (j *Journal) Append(event any) (seqNo, size int, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.seqNo++
	data, err := json.Marshal(event)
	if err != nil {
		return 0, 0, fmt.Errorf("terminal journal: marshal: %w", err)
	}
	if _, err := j.writer.Write(data); err != nil {
		return 0, 0, err
	}
	if err := j.writer.WriteByte('\n'); err != nil {
		return 0, 0, err
	}
	if err := j.writer.Flush(); err != nil {
		return 0, 0, err
	}
	return j.seqNo, len(data) + 1, nil
}

// Compact rewrites the journal without the lines drop answers true for,
// every other line kept in its order, and answers how many it dropped.
// Appends wait while it runs; with nothing to drop the file is left as it
// is.
func (j *Journal) Compact(drop func(line []byte) bool) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.writer.Flush(); err != nil {
		return 0, err
	}
	src, err := os.Open(j.path)
	if err != nil {
		return 0, fmt.Errorf("terminal journal: compact: %w", err)
	}
	tmp := j.path + ".compact"
	dst, err := os.Create(tmp)
	if err != nil {
		src.Close()
		return 0, fmt.Errorf("terminal journal: compact: %w", err)
	}
	dropped, err := copyKept(bufio.NewReader(src), dst, drop)
	src.Close()
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil || dropped == 0 {
		_ = os.Remove(tmp)
		return 0, err
	}
	// The file is closed before it is replaced (Windows refuses to rename
	// over an open file), then opened again for the next appends.
	_ = j.file.Close()
	rerr := os.Rename(tmp, j.path)
	if rerr != nil {
		_ = os.Remove(tmp)
		dropped = 0
	}
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return dropped, fmt.Errorf("terminal journal: reopen: %w", err)
	}
	j.file, j.writer = f, bufio.NewWriter(f)
	if rerr != nil {
		return 0, fmt.Errorf("terminal journal: compact: %w", rerr)
	}
	return dropped, nil
}

// copyKept copies each line of r that drop does not answer true for to w,
// a line of any length, and answers how many lines it dropped.
func copyKept(r *bufio.Reader, w io.Writer, drop func(line []byte) bool) (int, error) {
	bw := bufio.NewWriter(w)
	dropped := 0
	for {
		line, err := r.ReadBytes('\n')
		body := bytes.TrimSuffix(line, []byte("\n"))
		if len(body) > 0 {
			if drop(body) {
				dropped++
			} else if _, werr := bw.Write(append(body, '\n')); werr != nil {
				return 0, werr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	return dropped, bw.Flush()
}

// PanelEventKey identifies a journaled chat panel event: its panel, its
// sequence and its time - a panel opened again under the same id starts
// its sequence again, its time tells the two apart.
func PanelEventKey(ev PanelEvent) string {
	return panelKey(ev.Event.Panel, ev.Event.Seq, ev.Timestamp)
}

// PanelLineKey is the PanelEventKey of a journal line holding a chat panel
// event, "" for any other line.
func PanelLineKey(line []byte) string {
	if !bytes.Contains(line, []byte(`"panel.event"`)) {
		return ""
	}
	var probe struct {
		Type  string `json:"type"`
		Event struct {
			Panel string `json:"panel"`
			Seq   int64  `json:"seq"`
		} `json:"event"`
		Timestamp string `json:"ts"`
	}
	if json.Unmarshal(line, &probe) != nil || probe.Type != "panel.event" {
		return ""
	}
	return panelKey(probe.Event.Panel, probe.Event.Seq, probe.Timestamp)
}

func panelKey(panelID string, seq int64, ts string) string {
	return fmt.Sprintf("%s#%d@%s", panelID, seq, ts)
}

// Close flushes and closes the journal file.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	_ = j.writer.Flush()
	return j.file.Close()
}

// Replay reads all events from a journal file and calls fn for each.
func Replay(dir, sessionID string, fn func(line json.RawMessage) error) error {
	path := filepath.Join(dir, sessionID, "terminal.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no journal yet
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := fn(json.RawMessage(scanner.Bytes())); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// ReplayMessages reads all stored messages (msg.new events) from a journal.
func ReplayMessages(dir, sessionID string) ([]Message, error) {
	var messages []Message
	err := Replay(dir, sessionID, func(line json.RawMessage) error {
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			return nil // skip malformed lines
		}
		if probe.Type == "msg.new" {
			var evt MsgNewEvent
			if err := json.Unmarshal(line, &evt); err == nil {
				messages = append(messages, evt.Msg)
			}
		}
		return nil
	})
	return messages, err
}

// ReplayPanelEvents reads the chat panel events (panel.event) a journal
// holds, in their order, those a later replace makes stale left out
// (KeepPanelEvent).
func ReplayPanelEvents(dir, sessionID string) ([]PanelEvent, error) {
	var events []PanelEvent
	err := Replay(dir, sessionID, func(line json.RawMessage) error {
		var probe struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &probe) != nil || probe.Type != "panel.event" {
			return nil
		}
		var evt PanelEvent
		if json.Unmarshal(line, &evt) == nil {
			events = KeepPanelEvent(events, evt)
		}
		return nil
	})
	return events, err
}

// KeepPanelEvent appends ev to the panel events kept for a session's
// replay, dropping each earlier event of the same panel whose ops replace
// only nodes ev replaces too: a replace of a node makes the earlier ones
// stale, so a long run's replay holds each node's last state once.
func KeepPanelEvent(kept []PanelEvent, ev PanelEvent) []PanelEvent {
	targets := replacedNodes(ev)
	if len(targets) == 0 {
		return append(kept, ev)
	}
	out := kept[:0]
	for _, k := range kept {
		if k.Event.Panel == ev.Event.Panel && subsumed(replacedNodes(k), targets) {
			continue
		}
		out = append(out, k)
	}
	return append(out, ev)
}

// replacedNodes is the set of nodes an ops event replaces, nil when one of
// its ops is not a replace (or it is no ops event).
func replacedNodes(ev PanelEvent) map[string]bool {
	if len(ev.Event.Ops) == 0 {
		return nil
	}
	set := map[string]bool{}
	for _, op := range ev.Event.Ops {
		if op.Op != render.OpReplace || op.Target == "" {
			return nil
		}
		set[op.Target] = true
	}
	return set
}

func subsumed(nodes, by map[string]bool) bool {
	if len(nodes) == 0 {
		return false
	}
	for n := range nodes {
		if !by[n] {
			return false
		}
	}
	return true
}

// ReplayFrom reads events starting from a given sequence number.
func ReplayFrom(dir, sessionID string, fromSeqNo int, w io.Writer) error {
	return Replay(dir, sessionID, func(line json.RawMessage) error {
		var probe struct {
			SeqNo int `json:"seqNo"`
		}
		_ = json.Unmarshal(line, &probe)
		if probe.SeqNo >= fromSeqNo {
			_, err := w.Write(append(line, '\n'))
			return err
		}
		return nil
	})
}
