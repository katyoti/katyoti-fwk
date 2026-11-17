// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MarshalCanonicalJSON serializes the manifest into a byte-stable
// JSON form: 2-space indent, sorted map keys (Go's encoding/json
// already sorts map keys; arrays we sort at Build time). The
// trailing newline is included so the file plays nicely with
// version control and POSIX text-file conventions.
//
// Re-running BuildManifest + MarshalCanonicalJSON with identical
// inputs MUST produce byte-identical output — this is the
// idempotency guarantee that lets the drift detector compare on
// equality.
func (m Manifest) MarshalCanonicalJSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	// json.Encoder already appends "\n" — no need to add another.
	return buf.Bytes(), nil
}

// WriteManifestJSON renders the manifest and writes it to path,
// creating the parent directory if needed. Returns the number of
// bytes written.
//
// Atomic write: writes to a sibling .tmp file and renames into
// place so a crash mid-write leaves the previous manifest intact.
func WriteManifestJSON(m Manifest, path string) (int, error) {
	data, err := m.MarshalCanonicalJSON()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, fmt.Errorf("create manifest dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return 0, fmt.Errorf("write manifest tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		// Best-effort cleanup of the orphaned tmp file.
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("rename manifest into place: %w", err)
	}
	return len(data), nil
}
