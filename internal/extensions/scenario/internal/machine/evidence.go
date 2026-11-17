// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/redact"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/secrets"
)

// EvidenceDir is the folder of a case stage's work folder that keeps what the
// installed universe wrote during the stage, pass or fail: a failure on a
// clean machine explains itself from the results alone.
const EvidenceDir = "universe"

// evidenceMaxFile bounds one copied file: a longer one keeps its end (the
// last lines of a log say why the stage failed).
const evidenceMaxFile = 16 << 20

// evidenceMaxTotal bounds what one stage copies.
const evidenceMaxTotal = 256 << 20

// evidenceRecent are the universe data folders whose entries the stage wrote
// are kept: a task run's session journal (its children's too), its run
// record, the events.
var evidenceRecent = []string{"sessions", "runs", "events"}

// evidenceNeverDirs are folders never copied: raw tool results (a command's
// whole output) and key stores.
var evidenceNeverDirs = map[string]bool{"tool-results": true, "keys": true}

// evidenceNeverFiles are files never copied: the raw LLM call log holds the
// whole request and response bodies.
var evidenceNeverFiles = map[string]bool{"llm-model-calls.jsonl": true}

// evidenceNeverPrefixes are name prefixes of files and folders never copied:
// vaults, secret and token stores.
var evidenceNeverPrefixes = []string{"vault", "secret", "auth", "token", "credential"}

// evidence is one stage's copy: what is copied goes out of the machine, so
// secret files are never copied, every copied file is redacted, and the
// total is bounded.
type evidence struct {
	red  *redact.Redactor
	left int64
}

// saveEvidence copies into work/EvidenceDir what the universe whose data is
// data wrote: its logs folder, and each entry of evidenceRecent holding a
// file written since since. envFiles are the installation's and the
// universe's .env files: their values are read here, never written or
// logged, and masked in every copied file, whatever their key's name. Best
// effort - what cannot be read is skipped, the stage's verdict is never
// changed by it.
func saveEvidence(data, work string, since time.Time, envFiles []string) {
	if data == "" || work == "" {
		return
	}
	values := map[string]string{}
	for _, f := range envFiles {
		for k, v := range cenv.EnvFileValues(f) {
			values[k] = v
		}
	}
	ev := &evidence{red: redact.NewWithValues(redact.RedactionSettings{}, values), left: evidenceMaxTotal}
	dst := filepath.Join(work, EvidenceDir)
	ev.copy(filepath.Join(data, "logs"), filepath.Join(dst, "logs"))
	for _, name := range evidenceRecent {
		dir := filepath.Join(data, name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			src := filepath.Join(dir, e.Name())
			if writtenSince(src, since) {
				ev.copy(src, filepath.Join(dst, name, e.Name()))
			}
		}
	}
}

// evidenceEnvFiles are the .env files whose values a stage's evidence masks:
// the installation's, its global root's and the universe's.
func evidenceEnvFiles(inst *cenv.Installation, universeHome string) []string {
	var out []string
	if inst != nil {
		out = append(out, filepath.Join(inst.Dir(), ".env"), filepath.Join(inst.ConfigRoot(), ".env"))
	}
	if universeHome != "" {
		out = append(out, filepath.Join(application.EnvOf(universeHome).HomeDir(), ".env"))
	}
	return out
}

// evidenceClockSlack is how far before a stage's start a file's time may be
// and still count as written by the stage: a file system stamps files with
// the kernel's coarse clock (on Linux one tick, a few milliseconds behind
// time.Now()), so a file written just after the stage started can carry a
// time just before it. One second is larger than any file-system clock tick
// on Linux, Windows or macOS, and far below the age of a session from an
// earlier stage.
const evidenceClockSlack = time.Second

// writtenSince says whether path, or a file under it, was written at or
// after since, within evidenceClockSlack.
func writtenSince(path string, since time.Time) bool {
	since = since.Add(-evidenceClockSlack)
	found := false
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if info, err := d.Info(); err == nil && !info.ModTime().Before(since) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// evidenceNever says whether a file or folder named name is never copied.
func evidenceNever(name string, dir bool) bool {
	n := strings.ToLower(name)
	if dir && evidenceNeverDirs[n] || !dir && evidenceNeverFiles[n] {
		return true
	}
	if !dir && secrets.IsSecretPath(n) || dir && secrets.IsSecretPath(n+"/x") {
		return true
	}
	for _, p := range evidenceNeverPrefixes {
		if strings.HasPrefix(n, p) || strings.HasPrefix(n, "."+p) {
			return true
		}
	}
	return false
}

// copy copies the files of src (a file or a folder) into dst, keeping their
// relative paths, each one redacted.
func (ev *evidence) copy(src, dst string) {
	_ = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if evidenceNever(d.Name(), d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if ev.left <= 0 {
			return fs.SkipAll
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return nil
		}
		to := filepath.Join(dst, rel)
		if rel == "." {
			to = dst
		}
		body, ok := readTail(p, min(evidenceMaxFile, ev.left))
		if !ok {
			return nil
		}
		body = ev.red.RedactBytes(body)
		if os.MkdirAll(filepath.Dir(to), 0o755) == nil && os.WriteFile(to, body, 0o644) == nil {
			ev.left -= int64(len(body))
		}
		return nil
	})
}

// readTail reads the text file p, its last max bytes when it is longer -
// from the first whole line, after a line saying what was dropped. A binary
// file (a NUL in its first block) is not read: it cannot be redacted.
func readTail(p string, max int64) ([]byte, bool) {
	f, err := os.Open(p)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false
	}
	size := info.Size()
	head := make([]byte, min(size, 8<<10))
	if _, err := io.ReadFull(f, head); err != nil || bytes.IndexByte(head, 0) >= 0 {
		return nil, false
	}
	if size <= max {
		body, err := io.ReadAll(io.NewSectionReader(f, 0, size))
		return body, err == nil
	}
	body, err := io.ReadAll(io.NewSectionReader(f, size-max, max))
	if err != nil {
		return nil, false
	}
	if i := bytes.IndexByte(body, '\n'); i >= 0 {
		body = body[i+1:]
	}
	note := fmt.Sprintf("[evidence: the first %d bytes of this file were dropped, its end is kept]\n", size-int64(len(body)))
	return append([]byte(note), body...), true
}
