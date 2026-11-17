// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// THE QUARANTINE REACHES THE PERSON: the memory
// written before the wall waits in <root>/_quarantine/<stamp>-<source>/ until
// the person keeps it as their memory or deletes it. What waits is read from
// the folders each time it is asked - no state of its own - so a folder kept
// or deleted is gone from « À faire » at once (memory_waiting.go).

// quarantineMarkerFile is the small file quarantine writes in each folder it
// makes: where the memory came from and when. Never joined, never kept.
const quarantineMarkerFile = ".quarantine.json"

// quarantineMarker is what the marker holds.
type quarantineMarker struct {
	Source string `json:"source"`
	At     string `json:"at"`
}

func writeQuarantineMarker(dir string, m quarantineMarker) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, quarantineMarkerFile), data, 0o600)
}

// QuarantinedFolder is one folder of memory written before the wall.
type QuarantinedFolder struct {
	// Name is its folder's name under the quarantine: what keep and delete take.
	Name string `json:"name" jsonschema:"description=The quarantined folder's name: what keep and delete take"`
	// Source is the folder it was moved from.
	Source string `json:"source,omitempty" jsonschema:"description=The folder it was moved from"`
	// Since is when it was quarantined (RFC 3339).
	Since string   `json:"since,omitempty" jsonschema:"description=When it was quarantined (RFC 3339)"`
	Files []string `json:"files" jsonschema:"description=Its files relative to the folder"`
	Bytes int64    `json:"bytes" jsonschema:"description=Their size in bytes"`
}

// QuarantinedMemory lists the quarantined folders under root, oldest first;
// none when root is empty or has no quarantine.
func QuarantinedMemory(root string) ([]QuarantinedFolder, error) {
	if root == "" {
		return nil, nil
	}
	base := filepath.Join(root, quarantineDir)
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []QuarantinedFolder
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(base, e.Name())
		f := QuarantinedFolder{Name: e.Name(), Files: []string{}}
		if data, err := os.ReadFile(filepath.Join(dir, quarantineMarkerFile)); err == nil {
			var m quarantineMarker
			if json.Unmarshal(data, &m) == nil {
				f.Source, f.Since = m.Source, m.At
			}
		}
		if f.Source == "" {
			f.Source, f.Since = sourceOfName(e.Name())
		}
		files, size, _, err := quarantinedFiles(dir)
		if err != nil {
			return nil, err
		}
		f.Files, f.Bytes = files, size
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// sourceOfName reads a folder quarantined before the marker existed:
// <stamp>-<source with its separators made _>.
func sourceOfName(name string) (string, string) {
	stamp, rest, ok := strings.Cut(name, "-")
	if !ok {
		return name, ""
	}
	if t, err := time.Parse("20060102T150405Z", stamp); err == nil {
		return rest, t.UTC().Format(time.RFC3339)
	}
	return name, ""
}

// quarantinedFiles are a folder's regular files (its marker left out),
// slash-separated relative paths, and their size; odd are its entries that are
// neither a folder nor a regular file - a symbolic link, a junction, a device -
// never listed as memory and never followed: WalkDir does not
// descend a linked folder, and a link is not read through.
func quarantinedFiles(dir string) (files []string, size int64, odd []string, err error) {
	files = []string{}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if !d.Type().IsRegular() {
			odd = append(odd, filepath.ToSlash(rel))
			return nil
		}
		if rel == quarantineMarkerFile {
			return nil
		}
		if info, err := d.Info(); err == nil {
			size += info.Size()
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	sort.Strings(odd)
	return files, size, odd, err
}

// readQuarantinedFile reads a regular file of the quarantined folder r
// without following a link: every path component is resolved inside r
// (os.Root - a sub-folder swapped for a link or a junction after the walk does
// not lead out of the folder), the entry is Lstat-ed, opened, and the opened
// file must be that same regular file - an entry swapped for a link after the
// walk is refused - with no other name: a file whose link count is above 1 (a
// hard link, maybe to a file outside the quarantine) is refused, the count
// read on the opened file.
func readQuarantinedFile(r *os.Root, rel string) ([]byte, error) {
	name := filepath.FromSlash(rel)
	before, err := r.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("agent memory: %s cannot be read inside the quarantine (a link?): nothing was moved: %w", rel, err)
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("agent memory: %s is not a plain file (a link?): nothing was moved", rel)
	}
	f, err := r.Open(name)
	if err != nil {
		return nil, fmt.Errorf("agent memory: %s cannot be read inside the quarantine (a link?): nothing was moved: %w", rel, err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, fmt.Errorf("agent memory: %s changed while it was kept (a link?): nothing was moved", rel)
	}
	links, err := fileLinkCount(f, opened)
	if err != nil {
		return nil, fmt.Errorf("agent memory: %s: its link count cannot be read: nothing was moved: %w", rel, err)
	}
	if links > 1 {
		return nil, fmt.Errorf("agent memory: %s has %d names (a hard link?): delete the folder or remove it by hand first; nothing was moved", rel, links)
	}
	return io.ReadAll(f)
}

// readQuarantinedFiles reads the files (slash-separated, relative to dir)
// through an os.Root on dir, closed before it returns so the folder can go.
func readQuarantinedFiles(dir string, files []string) ([][]byte, error) {
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	contents := make([][]byte, len(files))
	for i, f := range files {
		data, err := readQuarantinedFile(r, f)
		if err != nil {
			return nil, err
		}
		contents[i] = data
	}
	return contents, nil
}

// quarantinedDir is the folder a name designates: exactly one existing folder
// directly under the quarantine. A name that is empty, climbs, holds a
// separator or is absolute is refused, as is one that names nothing.
func quarantinedDir(root, name string) (string, error) {
	if root == "" {
		return "", errors.New("agent memory: no memory root in this universe")
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\:`) ||
		filepath.IsAbs(name) || filepath.Base(name) != name || filepath.VolumeName(name) != "" {
		return "", fmt.Errorf("agent memory: %q is no quarantined folder's name", name)
	}
	dir := filepath.Join(root, quarantineDir, name)
	// Lstat, never Stat: an entry that is itself a link (or a Windows
	// junction) to a folder is no quarantined folder.
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return "", fmt.Errorf("agent memory: no quarantined folder %q", name)
	}
	return dir, nil
}

// AssignQuarantinedMemory keeps a quarantined folder as key's memory: its
// files move into the key's folder - a MEMORY.md already there keeps its text
// and gets the quarantined one appended - and the quarantine folder goes. A
// file of another name already in the key's folder refuses it, nothing moved;
// so does an entry that is not a plain file (a link is never followed out of
// the quarantine). Every file is read before any is written, so
// a refused read moves nothing.
func AssignQuarantinedMemory(root, name string, key MemoryKey) error {
	dir, err := quarantinedDir(root, name)
	if err != nil {
		return err
	}
	dest := NewMemoryManager(root, key).MemoryDir()
	if dest == "" {
		return errors.New("agent memory: no tenant, universe and person to keep it for")
	}
	files, _, odd, err := quarantinedFiles(dir)
	if err != nil {
		return err
	}
	if len(odd) > 0 {
		return fmt.Errorf("agent memory: %s is not a plain file (a link?): delete the folder or remove it by hand first; nothing was moved", strings.Join(odd, ", "))
	}
	source, _ := sourceOfFolder(dir, name)
	for _, f := range files {
		if f == memoryEntrypoint {
			continue
		}
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(f))); err == nil {
			return fmt.Errorf("agent memory: %s is already in your memory: delete it or move it by hand first; nothing was moved", f)
		}
	}
	contents, err := readQuarantinedFiles(dir, files)
	if err != nil {
		return err
	}
	for i, f := range files {
		to := filepath.Join(dest, filepath.FromSlash(f))
		data := contents[i]
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			return err
		}
		if f == memoryEntrypoint {
			if own, err := os.ReadFile(to); err == nil {
				data = []byte(strings.TrimRight(string(own), "\n") + "\n\n## Kept from " + source + "\n\n" + string(data))
			}
		}
		if err := os.WriteFile(to, data, 0o600); err != nil {
			return err
		}
	}
	return os.RemoveAll(dir)
}

// sourceOfFolder is where a quarantined folder came from.
func sourceOfFolder(dir, name string) (string, string) {
	if data, err := os.ReadFile(filepath.Join(dir, quarantineMarkerFile)); err == nil {
		var m quarantineMarker
		if json.Unmarshal(data, &m) == nil && m.Source != "" {
			return m.Source, m.At
		}
	}
	return sourceOfName(name)
}

// DeleteQuarantinedMemory removes a quarantined folder.
func DeleteQuarantinedMemory(root, name string) error {
	dir, err := quarantinedDir(root, name)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
