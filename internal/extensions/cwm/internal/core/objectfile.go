// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"gopkg.in/yaml.v3"
)

// One object, one file — in one of two spellings (concepts.md §5, amended
// 2026-09-24; phase 7c):
//
//   - `.cwm.yaml` — the whole object as YAML. For an object whose body is
//     data, or none.
//   - `.cwm.md`   — YAML frontmatter carries the object, the markdown body
//     carries `body` as PROSE. Nothing lives in both: the body is
//     stripped from the frontmatter, and the frontmatter never repeats the
//     body. The body is never parsed for data — it is a string to the index.
//
// THE MAPPING IS ONE LINE, THE SAME IN BOTH DIRECTIONS: the markdown ↔ the object's body.
// The spelling follows the object, not the type: WriteObjectFile writes
// what the path says, and objectFileNameFor picks `.cwm.md` for a prose body.

// ObjectsDirName is the default folder a module's objects live in; a type
// may declare another (`folder:`).
const ObjectsDirName = "objects"

// ObjectFileSuffix is the data spelling; api.ObjectFileSuffixMD the prose one.
const ObjectFileSuffix = api.ObjectFileSuffix

// fmBoundary matches a frontmatter fence line — the same rule as the
// frontmatter codec in internal/core/marshal.
var fmBoundary = regexp.MustCompile(`(?m)^-{3,}\s*$`)

// WriteObjectFile writes obj at path, in the spelling the path names.
func WriteObjectFile(path string, obj api.Object) error {
	_, err := writeObjectFileRev(path, obj)
	return err
}

// writeObjectFileRev is WriteObjectFile returning the revision of the bytes
// it wrote — what fileRevision would read back — so a write stamps its event
// without reading its own file again.
func writeObjectFileRev(path string, obj api.Object) (string, error) {
	data, err := encodeObjectFile(path, obj)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("cwm/core: object dir: %w", err)
	}
	if err := atomicio.WriteAtomic(path, data, 0o644); err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

// encodeObjectFile is the bytes WriteObjectFile writes at path: the spelling
// the path names, written through the file already there. The size budget
// measures these bytes.
func encodeObjectFile(path string, obj api.Object) ([]byte, error) {
	if obj.ID == "" {
		return nil, fmt.Errorf("%w: object ID required", api.ErrInvalidInput)
	}
	var data []byte
	if strings.HasSuffix(path, api.ObjectFileSuffixMD) {
		prose := obj.Body
		head := obj
		head.Body = "" // THE ONE LINE (writer): the body leaves the frontmatter for the markdown
		var prior []byte
		crlf := false
		if raw, err := atomicio.ReadAtomic(path); err == nil {
			crlf = strings.Contains(string(raw), "\r\n")
			if pfm, _, ok := splitObjectFrontmatter(string(raw)); ok {
				// The split leaves the newline that closed the opening fence.
				prior = []byte(strings.TrimPrefix(strings.TrimPrefix(pfm, "\r"), "\n"))
			}
		}
		fm, err := marshalObjectYAMLOver(prior, head)
		if err != nil {
			return nil, fmt.Errorf("cwm/core: marshal object %q: %w", obj.ID, err)
		}
		// Built in LF; a file checked out in CRLF stays in CRLF.
		lf := func(t string) string { return strings.ReplaceAll(t, "\r\n", "\n") }
		fmText, proseText := lf(string(fm)), lf(prose)
		var b strings.Builder
		b.WriteString("---\n")
		b.WriteString(fmText)
		if !strings.HasSuffix(fmText, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("---\n")
		b.WriteString(proseText)
		if proseText != "" && !strings.HasSuffix(proseText, "\n") {
			b.WriteString("\n")
		}
		out := b.String()
		if crlf {
			out = strings.ReplaceAll(out, "\n", "\r\n")
		}
		data = []byte(out)
	} else {
		prior, _ := atomicio.ReadAtomic(path) // absent = a new file; nothing to keep
		var err error
		data, err = marshalObjectYAMLOver(prior, obj)
		if err != nil {
			return nil, fmt.Errorf("cwm/core: marshal object %q: %w", obj.ID, err)
		}
	}
	return data, nil
}

// ReadObjectFile reads either spelling.
// A read retries through a concurrent write's rename (atomicio.ReadAtomic):
// on Windows a reader meeting the replace sees a sharing violation, or — inside
// the replace itself — the file missing; both are transient, not an answer.
func ReadObjectFile(path string) (api.Object, error) {
	raw, err := atomicio.ReadAtomic(path)
	if err != nil {
		return api.Object{}, fmt.Errorf("cwm/core: read object %s: %w", path, err)
	}
	return decodeObjectFile(path, raw)
}

// decodeObjectFile decodes an object file's bytes; path says its form
// (`.cwm.md` frontmatter + body, else YAML).
func decodeObjectFile(path string, raw []byte) (api.Object, error) {
	var obj api.Object
	if strings.HasSuffix(path, api.ObjectFileSuffixMD) {
		fm, body, ok := splitObjectFrontmatter(string(raw))
		if !ok {
			return api.Object{}, fmt.Errorf("%w: object file %s has no frontmatter fence", api.ErrInvalidInput, path)
		}
		if err := marshal.Unmarshal([]byte(fm), marshal.YAML, &obj); err != nil {
			return api.Object{}, fmt.Errorf("cwm/core: parse object %s: %w", path, err)
		}
		if obj.Body != "" {
			return api.Object{}, fmt.Errorf("%w: object file %s carries a body in BOTH the frontmatter and the markdown — nothing lives in both", api.ErrInvalidInput, path)
		}
		if strings.TrimSpace(body) != "" {
			obj.Body = strings.TrimLeft(body, "\n") // THE ONE LINE (reader): the markdown is the body
		}
	} else {
		if err := marshal.Unmarshal(raw, marshal.YAML, &obj); err != nil {
			return api.Object{}, fmt.Errorf("cwm/core: parse object %s: %w", path, err)
		}
	}
	if obj.ID == "" {
		return api.Object{}, fmt.Errorf("%w: object file %s declares no id", api.ErrInvalidInput, path)
	}
	return obj, nil
}

// splitObjectFrontmatter splits a `.cwm.md` file into its frontmatter and
// body. The fence must open the file.
func splitObjectFrontmatter(text string) (fm, body string, ok bool) {
	if !fmBoundary.MatchString(text) {
		return "", "", false
	}
	parts := fmBoundary.Split(text, 3)
	if len(parts) < 3 || strings.TrimSpace(parts[0]) != "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func cloneCaps(in map[string]map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(in))
	for k, v := range in {
		g := make(map[string]any, len(v))
		for kk, vv := range v {
			g[kk] = vv
		}
		out[k] = g
	}
	return out
}

// marshalObjectYAML writes an object as YAML at TWO-SPACE indent — the
// corpus's hand-written convention, which the probes' regexes read
// (`^\s{2}container:`). yaml.v3's default of four made every machine-written
// object invisible to the grader (found 2026-09-25 on the migrated decisions).
func marshalObjectYAML(obj api.Object) ([]byte, error) {
	n, err := objectNode(obj)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// marshalObjectYAMLOver writes obj THROUGH the file already there, so a write
// changes what changed and nothing else (found 2026-09-25 in the running GUI:
// adding an aspect rewrote a module object from the struct and erased its
// header note and an inline `# ` remark — every GUI edit did that).
//
// The prior YAML is parsed as a node tree and the object's new encoding merged
// into it: a node whose VALUE is unchanged is kept as it was — its comments,
// its `>-` folding, its key order; a changed value is replaced and inherits
// the comments of the one it replaces; a new key is appended; a removed key
// goes — but a key this binary does not know is kept (keepUnknownKeys). The text itself is kept by splicing (objectfile_splice.go); the node
// merge is the fallback when the prior text is not a plain block mapping. No
// prior file, or one that does not parse as a mapping, falls back to the plain
// encoding — a write never fails because of what it replaces.
func marshalObjectYAMLOver(prior []byte, obj api.Object) ([]byte, error) {
	if len(bytes.TrimSpace(prior)) == 0 {
		return marshalObjectYAML(obj)
	}
	var old yaml.Node
	if yaml.Unmarshal(prior, &old) != nil || old.Kind != yaml.DocumentNode || len(old.Content) == 0 || old.Content[0].Kind != yaml.MappingNode {
		return marshalObjectYAML(obj)
	}
	fresh, err := objectNode(obj)
	if err != nil {
		return nil, err
	}
	keepUnknownKeys(old.Content[0], fresh)
	if out, ok := spliceObjectYAML(prior, fresh); ok {
		return out, nil
	}
	old.Content[0] = mergeYAMLNode(old.Content[0], fresh)
	keepTextsWhole(&old) // a kept node is re-emitted too
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(&old); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// objectNode is obj as the node tree every object write emits — the plain
// encoding, the splice and the merge alike, so the size budget measures the
// bytes a write leaves.
//
// yaml.v3 writes a multi-line string as a literal block, and that block does
// not always read back: a leading newline is dropped ("\nab" is written `|2-`
// and read "ab" — a markdown section snapshot often starts with one), and a
// tab-led line is written in a form its own parser refuses. Node.Encode goes
// through that writer, so such a text is encoded as a stand-in and put back,
// double-quoted, in the node. Every other multi-line text keeps its readable
// block.
func objectNode(obj api.Object) (*yaml.Node, error) {
	texts := map[string]string{}
	stood := withStandIns(reflect.ValueOf(obj), texts)
	var n yaml.Node
	if err := n.Encode(stood.Interface()); err != nil {
		return nil, err
	}
	restoreTexts(&n, texts)
	return &n, nil
}

// standInPrefix opens a stand-in; U+2063 (invisible separator) keeps it apart
// from any authored text.
const standInPrefix = "\u2063cwm-text-"

// withStandIns returns a copy of v in which every string the literal block
// would not give back is replaced by a stand-in, recorded in texts.
func withStandIns(v reflect.Value, texts map[string]string) reflect.Value {
	switch v.Kind() {
	case reflect.String:
		if s := v.String(); textBreaksBlock(s) {
			stand := fmt.Sprintf("%s%d\u2063", standInPrefix, len(texts))
			texts[stand] = s
			return reflect.ValueOf(stand).Convert(v.Type())
		}
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return v
		}
		if v.Kind() == reflect.Pointer {
			out := reflect.New(v.Type().Elem())
			out.Elem().Set(withStandIns(v.Elem(), texts))
			return out
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(withStandIns(v.Elem(), texts))
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		for it := v.MapRange(); it.Next(); {
			out.SetMapIndex(it.Key(), withStandIns(it.Value(), texts))
		}
		return out
	case reflect.Slice:
		if v.IsNil() || v.Type().Elem().Kind() == reflect.Uint8 {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(withStandIns(v.Index(i), texts))
		}
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := 0; i < v.NumField(); i++ {
			if f := out.Field(i); f.CanSet() {
				f.Set(withStandIns(v.Field(i), texts))
			}
		}
		return out
	}
	return v
}

// restoreTexts puts each stand-in's text back, double-quoted.
func restoreTexts(n *yaml.Node, texts map[string]string) {
	if len(texts) == 0 || n == nil {
		return
	}
	if n.Kind == yaml.ScalarNode {
		if s, ok := texts[n.Value]; ok {
			n.Value, n.Tag, n.Style = s, "!!str", yaml.DoubleQuotedStyle
		}
		return
	}
	for _, c := range n.Content {
		restoreTexts(c, texts)
	}
}

// keepTextsWhole double-quotes, in a node tree read from a file, every
// multi-line string the literal block would not give back — a kept node is
// re-emitted too.
func keepTextsWhole(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.ScalarNode {
		if n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle) == 0 && n.ShortTag() == "!!str" && strings.Contains(n.Value, "\n") && !scalarRoundTrips(n) {
			n.Style = yaml.DoubleQuotedStyle
		}
		return
	}
	for _, c := range n.Content {
		keepTextsWhole(c)
	}
}

// textBreaksBlock reports whether s is a multi-line string yaml.v3 would not
// read back as it writes it.
func textBreaksBlock(s string) bool {
	return strings.Contains(s, "\n") && !scalarRoundTrips(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s})
}

// scalarRoundTrips reports whether n, emitted as a mapping value in its
// current style, reads back as its value.
func scalarRoundTrips(n *yaml.Node) bool {
	v := *n
	v.HeadComment, v.LineComment, v.FootComment = "", "", ""
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "k"}, &v}}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if enc.Encode(m) != nil || enc.Close() != nil {
		return false
	}
	var back map[string]string
	return yaml.Unmarshal(b.Bytes(), &back) == nil && back["k"] == n.Value
}

// objectKeys and edgeKeys are the keys this binary's api.Object and
// api.ObjectEdge know — read from their yaml tags, so a field added to either
// is known at once.
var (
	objectKeys = yamlKeys(reflect.TypeOf(api.Object{}))
	edgeKeys   = yamlKeys(reflect.TypeOf(api.ObjectEdge{}))
)

func yamlKeys(t reflect.Type) map[string]bool {
	keys := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		if name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ","); name != "" && name != "-" {
			keys[name] = true
		}
	}
	return keys
}

// keepUnknownKeys carries into fresh what the file holds and this binary does
// not know: a top-level key api.Object has no field for, and an edge's key
// api.ObjectEdge has none for (the edge matched by kind and target). A newer
// binary may have written them; the decode dropped them, and without this an
// older binary's next write would delete them. A key the struct knows still
// goes when the update clears it — only what this binary cannot see is kept.
func keepUnknownKeys(old, fresh *yaml.Node) {
	if old.Kind != yaml.MappingNode || fresh.Kind != yaml.MappingNode {
		return
	}
	freshAt := map[string]int{}
	for i := 0; i+1 < len(fresh.Content); i += 2 {
		freshAt[fresh.Content[i].Value] = i + 1
	}
	for i := 0; i+1 < len(old.Content); i += 2 {
		k, v := old.Content[i], old.Content[i+1]
		switch {
		case !objectKeys[k.Value]:
			if _, ok := freshAt[k.Value]; !ok {
				fresh.Content = append(fresh.Content, k, v)
			}
		case k.Value == "edges":
			if j, ok := freshAt["edges"]; ok {
				keepUnknownEdgeKeys(v, fresh.Content[j])
			}
		}
	}
}

// keepUnknownEdgeKeys copies, into each fresh edge, the unknown keys of the old
// edge with the same kind and target.
func keepUnknownEdgeKeys(old, fresh *yaml.Node) {
	if old.Kind != yaml.SequenceNode || fresh.Kind != yaml.SequenceNode {
		return
	}
	extras := map[string][]*yaml.Node{}
	for _, e := range old.Content {
		if e.Kind != yaml.MappingNode {
			continue
		}
		var more []*yaml.Node
		for i := 0; i+1 < len(e.Content); i += 2 {
			if !edgeKeys[e.Content[i].Value] {
				more = append(more, e.Content[i], e.Content[i+1])
			}
		}
		if len(more) > 0 {
			extras[edgeIdentity(e)] = more
		}
	}
	if len(extras) == 0 {
		return
	}
	for _, e := range fresh.Content {
		if more, ok := extras[edgeIdentity(e)]; ok && e.Kind == yaml.MappingNode {
			e.Content = append(e.Content, more...)
		}
	}
}

func edgeIdentity(e *yaml.Node) string {
	var kind, to string
	for i := 0; i+1 < len(e.Content); i += 2 {
		switch e.Content[i].Value {
		case "kind":
			kind = e.Content[i+1].Value
		case "to":
			to = e.Content[i+1].Value
		}
	}
	return kind + "\x00" + to
}

// mergeYAMLNode returns the node to write for a value: old when it means the
// same, a merge of the two for mappings, new (carrying old's comments) else.
func mergeYAMLNode(old, fresh *yaml.Node) *yaml.Node {
	if sameYAMLValue(old, fresh) {
		return old
	}
	if old.Kind == yaml.MappingNode && fresh.Kind == yaml.MappingNode {
		freshVal := map[string]*yaml.Node{}
		for i := 0; i+1 < len(fresh.Content); i += 2 {
			freshVal[fresh.Content[i].Value] = fresh.Content[i+1]
		}
		var content []*yaml.Node
		kept := map[string]bool{}
		for i := 0; i+1 < len(old.Content); i += 2 {
			k := old.Content[i]
			nv, ok := freshVal[k.Value]
			if !ok {
				continue // removed
			}
			kept[k.Value] = true
			content = append(content, k, mergeYAMLNode(old.Content[i+1], nv))
		}
		for i := 0; i+1 < len(fresh.Content); i += 2 {
			if !kept[fresh.Content[i].Value] {
				content = append(content, fresh.Content[i], fresh.Content[i+1]) // added
			}
		}
		old.Content = content
		return old
	}
	if fresh.HeadComment == "" {
		fresh.HeadComment = old.HeadComment
	}
	if fresh.LineComment == "" {
		fresh.LineComment = old.LineComment
	}
	if fresh.FootComment == "" {
		fresh.FootComment = old.FootComment
	}
	return fresh
}

// sameYAMLValue compares what two nodes MEAN, not how they are spelled.
func sameYAMLValue(a, b *yaml.Node) bool {
	var va, vb any
	if a.Decode(&va) != nil || b.Decode(&vb) != nil {
		return false
	}
	ja, errA := json.Marshal(va)
	jb, errB := json.Marshal(vb)
	return errA == nil && errB == nil && bytes.Equal(ja, jb)
}
