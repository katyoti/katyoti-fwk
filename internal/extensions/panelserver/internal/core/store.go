// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
)

// The files of one panel in the server's folder.
const (
	logExt      = ".jsonl"
	snapshotExt = ".snapshot.json"
)

// store is the server's folder: one log and one snapshot per panel. Only the
// server writes it, one panel at a time under that panel's lock.
type store struct{ dir string }

func (st store) logPath(id string) string      { return filepath.Join(st.dir, id+logExt) }
func (st store) snapshotPath(id string) string { return filepath.Join(st.dir, id+snapshotExt) }

// appendRecord adds one line to the panel's log.
func (st store) appendRecord(id string, r panelapi.Record) error {
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("panel server: %s: %w", id, err)
	}
	if err := os.MkdirAll(st.dir, 0o700); err != nil {
		return fmt.Errorf("panel server: %w", err)
	}
	f, err := os.OpenFile(st.logPath(id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("panel server: %s: %w", id, err)
	}
	_, werr := f.Write(append(line, '\n'))
	cerr := f.Close()
	if werr != nil {
		return fmt.Errorf("panel server: %s: %w", id, werr)
	}
	return cerr
}

// readLog reads the panel's log, every line in order; a line that does not
// read (a write cut short) is skipped. No log: none and no error.
func (st store) readLog(id string) ([]panelapi.Record, error) {
	data, err := atomicio.ReadAtomic(st.logPath(id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("panel server: %s: %w", id, err)
	}
	var out []panelapi.Record
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var r panelapi.Record
		if json.Unmarshal(line, &r) != nil || (r.Event == nil && r.Input == nil) {
			continue
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// writeSnapshot replaces the panel's snapshot.
func (st store) writeSnapshot(s panelapi.Snapshot) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("panel server: %s: %w", s.Panel, err)
	}
	if err := os.MkdirAll(st.dir, 0o700); err != nil {
		return fmt.Errorf("panel server: %w", err)
	}
	return atomicio.WriteAtomic(st.snapshotPath(s.Panel), data, 0o600)
}

// readSnapshot reads the panel's snapshot as last written; ok false when
// there is none or it does not read.
func (st store) readSnapshot(id string) (panelapi.Snapshot, bool) {
	data, err := atomicio.ReadAtomic(st.snapshotPath(id))
	if err != nil {
		return panelapi.Snapshot{}, false
	}
	var s panelapi.Snapshot
	if json.Unmarshal(data, &s) != nil || s.Panel != id {
		return panelapi.Snapshot{}, false
	}
	return s, true
}

// load rebuilds the panel from its files: the snapshot as last written, then
// the log's lines it does not hold yet (the snapshot is written after the
// log). The log alone rebuilds it when the snapshot is missing.
func (st store) load(id string) (panelapi.Snapshot, bool, error) {
	s, ok := st.readSnapshot(id)
	records, err := st.readLog(id)
	if err != nil {
		return panelapi.Snapshot{}, false, err
	}
	if !ok {
		if len(records) == 0 {
			return panelapi.Snapshot{}, false, nil
		}
		s = panelapi.Snapshot{Panel: id}
	}
	inputs := 0
	for _, r := range records {
		switch {
		case r.Event != nil && r.Event.Seq > s.Seq:
			fold(&s, *r.Event, r.At)
		case r.Input != nil:
			inputs++
			if inputs > s.Inputs {
				take(&s, r.At)
			}
		}
	}
	return s, true, nil
}

// ids lists the panels the folder holds, by their log.
func (st store) ids() ([]string, error) {
	entries, err := os.ReadDir(st.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("panel server: %w", err)
	}
	var out []string
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), logExt)
		if ok && !e.IsDir() && panel.ValidID(id) {
			out = append(out, id)
		}
	}
	return out, nil
}

// remove deletes the panel's files.
func (st store) remove(id string) error {
	var errs []error
	for _, p := range []string{st.logPath(id), st.snapshotPath(id)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// expired reports whether the panel's retention has passed at now: from its
// end, or - a panel whose run never ended it - from its last change. A panel
// waiting for an answer never expires: it is kept until it is answered.
func expired(s panelapi.Snapshot, retention time.Duration, now time.Time) bool {
	if s.Waiting {
		return false
	}
	from := s.Ended
	if from.IsZero() {
		from = s.Updated
	}
	return !from.IsZero() && now.Sub(from) > retention
}
