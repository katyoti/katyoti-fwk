// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
	"github.com/katyoti/katyoti-fwk/tools/secret-scan/secretscan"
)

// THE SHARDED TREE - an export's tree holds one file per
// record, so an export that runs every few minutes writes only the blobs that
// changed: store.json (the repository and the ticket sequence),
// todos/<module>/<yyyymm>/<to-do id>.json (the to-do's row and every revision
// kept of it; the month is the one its id was made in) and
// waits/<module>.jsonl (the module's waits). The blob and tree ids are
// computed here; only the objects the ref's last tree does not hold are
// written - the blobs in one hash-object, the trees in one mktree --batch.
// An export written before the sharding (todos.jsonl, revisions.jsonl and
// waits.jsonl at the root) is still read back.

// The folders of a sharded tree.
const (
	exportTodosDir = "todos"
	exportWaitsDir = "waits"
)

// exportFormat is store.json's format of the sharded tree.
const exportFormat = 2

// exportHead is store.json: the dump's header and the tree's format.
type exportHead struct {
	sqlstore.Dump
	Format int `json:"format,omitempty"`
}

// exportRecord is a to-do's file: its row and its revisions.
type exportRecord struct {
	Todo      sqlstore.TodoRow       `json:"todo"`
	Revisions []sqlstore.RevisionRow `json:"revisions,omitempty"`
}

var todoMonth = regexp.MustCompile(`TD-(\d{6})`)

// shardName is a path segment made of a name: never empty, never a separator.
func shardName(s string) string {
	s = strings.NewReplacer("/", "_", "\\", "_").Replace(s)
	if s == "" || s == "." || s == ".." {
		return "_"
	}
	return s
}

// todoPath is where a to-do's file goes in the tree.
func todoPath(r sqlstore.TodoRow) string {
	month := "other"
	if m := todoMonth.FindStringSubmatch(r.ID); m != nil {
		month = m[1]
	}
	return exportTodosDir + "/" + shardName(r.Module) + "/" + month + "/" + shardName(r.ID) + ".json"
}

// encodeJSON is one JSON value and a new line, HTML left as it is.
func encodeJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// exportFiles is a dump as its tree's files, by path: the same rows give the
// same files.
func exportFiles(d sqlstore.Dump) (map[string][]byte, error) {
	files := map[string][]byte{}
	head, err := encodeJSON(exportHead{Dump: d, Format: exportFormat})
	if err != nil {
		return nil, err
	}
	files[exportStoreFile] = head
	revs := map[string][]sqlstore.RevisionRow{}
	for _, r := range d.Revisions {
		revs[r.ID] = append(revs[r.ID], r)
	}
	for _, t := range d.Todos {
		p := todoPath(t)
		if _, dup := files[p]; dup {
			return nil, fmt.Errorf("two to-dos go to %s", p)
		}
		if files[p], err = encodeJSON(exportRecord{Todo: t, Revisions: revs[t.ID]}); err != nil {
			return nil, err
		}
	}
	waits := map[string][]sqlstore.WaitRow{}
	for _, w := range d.Waits {
		p := exportWaitsDir + "/" + shardName(w.Module) + ".jsonl"
		waits[p] = append(waits[p], w)
	}
	for p, ws := range waits {
		if files[p], err = jsonLines(ws); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// --- the git objects -----------------------------------------------------------

// objectHasher hashes git objects in the repository's object format.
type objectHasher func() hash.Hash

func hasherOf(dir string) (objectHasher, error) {
	out, err := gitcli.Run(dir, "rev-parse", "--show-object-format")
	if err != nil {
		return nil, err
	}
	switch f := strings.TrimSpace(out); f {
	case "", "sha1":
		return sha1.New, nil
	case "sha256":
		return sha256.New, nil
	default:
		return nil, fmt.Errorf("object format %q", f)
	}
}

// objectID is the id git gives an object of that kind and content.
func (h objectHasher) objectID(kind string, content []byte) []byte {
	w := h()
	w.Write([]byte(kind + " " + strconv.Itoa(len(content)) + "\x00"))
	w.Write(content)
	return w.Sum(nil)
}

// treeNode is a folder of the tree being built.
type treeNode struct {
	blobs map[string][]byte // name -> raw id
	dirs  map[string]*treeNode
}

func newTreeNode() *treeNode {
	return &treeNode{blobs: map[string][]byte{}, dirs: map[string]*treeNode{}}
}

// pendingTree is a tree object to write: its mktree line text and its id.
type pendingTree struct {
	text string
	id   string
}

// build computes the node's tree id; the trees not in have are appended to
// out, children first.
func (n *treeNode) build(h objectHasher, have map[string]bool, out *[]pendingTree) []byte {
	type entry struct {
		name, sortKey, mode, kind string
		id                        []byte
	}
	var es []entry
	for name, id := range n.blobs {
		es = append(es, entry{name, name, "100644", "blob", id})
	}
	for name, d := range n.dirs {
		es = append(es, entry{name, name + "/", "40000", "tree", d.build(h, have, out)})
	}
	sort.Slice(es, func(i, j int) bool { return es[i].sortKey < es[j].sortKey })
	var raw, text bytes.Buffer
	for _, e := range es {
		raw.WriteString(e.mode + " " + e.name + "\x00")
		raw.Write(e.id)
		fmt.Fprintf(&text, "%s %s %s\t%s\n", e.mode, e.kind, hex.EncodeToString(e.id), e.name)
	}
	id := h.objectID("tree", raw.Bytes())
	if hexID := hex.EncodeToString(id); !have[hexID] {
		have[hexID] = true
		*out = append(*out, pendingTree{text: text.String(), id: hexID})
	}
	return id
}

// treeObjects is every object id the tree holds, itself included ("" none).
func treeObjects(dir, tree string) (map[string]bool, error) {
	have := map[string]bool{}
	if tree == "" {
		return have, nil
	}
	have[tree] = true
	out, err := gitcli.Run(dir, "ls-tree", "-r", "-t", "--format=%(objectname)", tree)
	if err != nil {
		return nil, err
	}
	for _, l := range gitcli.Lines(out) {
		have[l] = true
	}
	return have, nil
}

// writeTree writes the files as a tree - only the objects base (a tree id, ""
// none) does not hold - and answers its id.
func writeTree(dir string, files map[string][]byte, base string) (string, error) {
	h, err := hasherOf(dir)
	if err != nil {
		return "", err
	}
	have, err := treeObjects(dir, base)
	if err != nil {
		return "", err
	}
	root := newTreeNode()
	var blobs []string // paths of the blobs to write
	for p, content := range files {
		id := h.objectID("blob", content)
		parts := strings.Split(p, "/")
		n := root
		for _, d := range parts[:len(parts)-1] {
			next, ok := n.dirs[d]
			if !ok {
				next = newTreeNode()
				n.dirs[d] = next
			}
			n = next
		}
		n.blobs[parts[len(parts)-1]] = id
		if hexID := hex.EncodeToString(id); !have[hexID] {
			have[hexID] = true
			blobs = append(blobs, p)
		}
	}
	if err := scanBlobs(files, blobs); err != nil {
		return "", err
	}
	if err := writeBlobs(dir, files, blobs); err != nil {
		return "", err
	}
	var trees []pendingTree
	rootID := hex.EncodeToString(root.build(h, have, &trees))
	if len(trees) == 0 {
		return rootID, nil
	}
	var in bytes.Buffer
	for _, t := range trees {
		in.WriteString(t.text)
		in.WriteString("\n")
	}
	out, err := gitcli.RunInput(dir, in.Bytes(), "mktree", "--batch")
	if err != nil {
		return "", err
	}
	got := gitcli.Lines(out)
	if len(got) != len(trees) {
		return "", fmt.Errorf("mktree --batch wrote %d trees, want %d", len(got), len(trees))
	}
	for i, t := range trees {
		if got[i] != t.id {
			return "", fmt.Errorf("mktree wrote %s, computed %s", got[i], t.id)
		}
	}
	return rootID, nil
}

// ErrSecretInExport refuses an export whose files hold a secret.
var ErrSecretInExport = errors.New("the export holds a secret")

// exportShapes is the secret scanner's content check, built once.
var exportShapes = sync.OnceValue(secretscan.NewShapes)

// scanBlobs refuses the export when a blob it is about to write - a file the
// ref's last tree does not hold - carries a secret: the
// scanner runs in process on the contents, before any object is written and
// so before the ref moves, in every repository, hook or no hook. A finding
// names the path, the line and the kind, never the value.
func scanBlobs(files map[string][]byte, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	written := make(map[string][]byte, len(paths))
	for _, p := range paths {
		written[p] = files[p]
	}
	findings := secretscan.ScanFiles(written, exportShapes())
	if len(findings) == 0 {
		return nil
	}
	names := make([]string, len(findings))
	for i, f := range findings {
		names[i] = f.String()
	}
	return fmt.Errorf("%w: %s", ErrSecretInExport, strings.Join(names, "; "))
}

// writeBlobs writes the files named into the repository's objects, in one
// hash-object through a temporary folder.
func writeBlobs(dir string, files map[string][]byte, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	tmp, err := os.MkdirTemp("", "katyoti-export-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	var list bytes.Buffer
	for i, p := range paths {
		f := filepath.Join(tmp, strconv.Itoa(i))
		if err := os.WriteFile(f, files[p], 0o600); err != nil {
			return err
		}
		list.WriteString(filepath.ToSlash(f) + "\n")
	}
	out, err := gitcli.RunInput(dir, list.Bytes(), "hash-object", "-w", "--no-filters", "--stdin-paths")
	if err != nil {
		return err
	}
	if n := len(gitcli.Lines(out)); n != len(paths) {
		return fmt.Errorf("hash-object wrote %d blobs, want %d", n, len(paths))
	}
	return nil
}

// --- reading it back -------------------------------------------------------------

// treeBlobs reads every blob of a commit's tree, by path, in one cat-file.
func treeBlobs(dir, commit string) (map[string][]byte, error) {
	out, err := gitcli.Run(dir, "ls-tree", "-r", "--format=%(objectname) %(path)", commit)
	if err != nil {
		return nil, err
	}
	var ids, paths []string
	for _, l := range strings.Split(out, "\n") {
		id, p, ok := strings.Cut(l, " ")
		if !ok {
			continue
		}
		ids, paths = append(ids, id), append(paths, p)
	}
	if len(ids) == 0 {
		return map[string][]byte{}, nil
	}
	raw, err := gitcli.RunInput(dir, []byte(strings.Join(ids, "\n")+"\n"), "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	rest := []byte(raw)
	for i := range ids {
		nl := bytes.IndexByte(rest, '\n')
		if nl < 0 {
			return nil, fmt.Errorf("cat-file: no header for %s", paths[i])
		}
		f := strings.Fields(string(rest[:nl]))
		if len(f) != 3 {
			return nil, fmt.Errorf("cat-file: %q for %s", rest[:nl], paths[i])
		}
		size, err := strconv.Atoi(f[2])
		if err != nil || nl+1+size > len(rest) {
			return nil, fmt.Errorf("cat-file: size %q for %s", f[2], paths[i])
		}
		files[paths[i]] = rest[nl+1 : nl+1+size]
		rest = rest[nl+1+size:]
		if len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
	}
	return files, nil
}

// dumpOfTree reads a dump back from its tree's files - sharded, or the three
// whole files of an export written before the sharding.
func dumpOfTree(files map[string][]byte) (sqlstore.Dump, error) {
	var head exportHead
	raw, ok := files[exportStoreFile]
	if !ok {
		return sqlstore.Dump{}, fmt.Errorf("no %s", exportStoreFile)
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return sqlstore.Dump{}, fmt.Errorf("%s: %w", exportStoreFile, err)
	}
	d := head.Dump
	if _, whole := files[exportTodosFile]; whole {
		read := func(name string) ([]byte, error) { return files[name], nil }
		if err := readLines(read, exportTodosFile, &d.Todos); err != nil {
			return d, err
		}
		if err := readLines(read, exportRevisionsFile, &d.Revisions); err != nil {
			return d, err
		}
		if err := readLines(read, exportWaitsFile, &d.Waits); err != nil {
			return d, err
		}
		return d, nil
	}
	for p, content := range files {
		switch {
		case strings.HasPrefix(p, exportTodosDir+"/"):
			var r exportRecord
			if err := json.Unmarshal(content, &r); err != nil {
				return d, fmt.Errorf("%s: %w", p, err)
			}
			d.Todos = append(d.Todos, r.Todo)
			d.Revisions = append(d.Revisions, r.Revisions...)
		case strings.HasPrefix(p, exportWaitsDir+"/"):
			read := func(string) ([]byte, error) { return content, nil }
			if err := readLines(read, p, &d.Waits); err != nil {
				return d, err
			}
		}
	}
	// The dump's order: to-dos and waits by id, revisions by id then revision.
	sort.Slice(d.Todos, func(i, j int) bool { return d.Todos[i].ID < d.Todos[j].ID })
	sort.Slice(d.Waits, func(i, j int) bool { return d.Waits[i].ID < d.Waits[j].ID })
	sort.Slice(d.Revisions, func(i, j int) bool {
		a, b := d.Revisions[i], d.Revisions[j]
		return a.ID < b.ID || a.ID == b.ID && a.Rev < b.Rev
	})
	return d, nil
}
