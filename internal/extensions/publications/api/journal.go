// SPDX-License-Identifier: Apache-2.0

package api

// THE CONNECTOR-WRITE JOURNAL: every write a publication
// connector makes - or refuses - is appended as one JSON line to a file of
// the tenant's data folder, the condition for writing to a shared space until
// an audit stream takes it over.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// JournalFileName is the journal's file under the tenant data folder's publications
// folder.
const JournalFileName = "connector-writes.jsonl"

// JournalPath is the journal of the tenant whose data folder is tenantDataDir
// (application.Env().TenantDataDir()); empty when there is none.
func JournalPath(tenantDataDir string) string {
	if tenantDataDir == "" {
		return ""
	}
	return filepath.Join(tenantDataDir, "publications", JournalFileName)
}

var journalMu sync.Mutex

// AppendWrite adds one write to the journal at path, stamped now when it carries
// no time. Appends of one process never interleave.
func AppendWrite(path string, w ConnectorWrite) error {
	if path == "" {
		return errors.New("publications: no connector-write journal (no tenant data folder)")
	}
	if w.At == "" {
		w.At = time.Now().UTC().Format(time.RFC3339Nano)
	}
	line, err := json.Marshal(w)
	if err != nil {
		return err
	}
	journalMu.Lock()
	defer journalMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("publications: the connector-write journal: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("publications: the connector-write journal: %w", err)
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// ReadWrites is every write of the journal at path, in order; none when it does
// not exist.
func ReadWrites(path string) ([]ConnectorWrite, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []ConnectorWrite
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var w ConnectorWrite
		if err := json.Unmarshal(sc.Bytes(), &w); err != nil {
			return out, fmt.Errorf("publications: %s: %w", path, err)
		}
		out = append(out, w)
	}
	return out, sc.Err()
}
