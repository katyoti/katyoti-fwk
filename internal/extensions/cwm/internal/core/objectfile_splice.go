// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

// SPLICING A WRITE INTO THE AUTHORED TEXT. Re-encoding the merged node tree
// keeps comments and key order but not the text: a `>-` block is refolded onto
// one line and a comment's padding collapses (`none # ` → `none # `).
// So the write goes through the TEXT: an entry whose value did not change keeps
// its lines byte for byte; a changed entry is rendered in place at its indent
// (a mapping on both sides is spliced entry by entry, recursively); a removed
// entry goes with its head comments; a new one is appended to its mapping.
// The result is parsed back and must mean the new object — otherwise the
// caller falls back to the node encoding.

// spliceObjectYAML returns prior with fresh's values spliced in, or ok=false
// when the prior text is not a plain block mapping this can edit safely.
func spliceObjectYAML(prior []byte, fresh *yaml.Node) (out []byte, ok bool) {
	crlf := bytes.Contains(prior, []byte("\r\n"))
	text := string(prior)
	if crlf {
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	var doc yaml.Node
	if yaml.Unmarshal([]byte(text), &doc) != nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, false
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode || root.Style&yaml.FlowStyle != 0 || fresh.Kind != yaml.MappingNode {
		return nil, false
	}
	trailingNL := strings.HasSuffix(text, "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	blocks := blockScalars(lines, root)
	spliced, ok := spliceMapping(lines, root, fresh, 0, len(lines), 0, blocks)
	if !ok {
		return nil, false
	}
	res := strings.Join(spliced, "\n")
	if trailingNL {
		res += "\n"
	}
	// The splice must mean the new object.
	var back yaml.Node
	if yaml.Unmarshal([]byte(res), &back) != nil || len(back.Content) != 1 || !sameYAMLValue(back.Content[0], fresh) {
		return nil, false
	}
	if crlf {
		res = strings.ReplaceAll(res, "\n", "\r\n")
	}
	return []byte(res), true
}

// spliceMapping rewrites lines[start:end), the text of the block mapping old
// whose keys sit at column indent, so it holds fresh.
func spliceMapping(lines []string, old, fresh *yaml.Node, start, end, indent int, blocks map[string]blockText) ([]string, bool) {
	n := len(old.Content) / 2
	if n == 0 || old.Style&yaml.FlowStyle != 0 {
		return nil, false
	}
	keyLine := make([]int, n) // 0-based
	for i := 0; i < n; i++ {
		k := old.Content[2*i]
		if k.Column-1 != indent || k.Line-1 < start || k.Line-1 >= end {
			return nil, false
		}
		keyLine[i] = k.Line - 1
	}
	// An entry starts at its head comments: the comment / blank lines right
	// above its key belong to it, not to the entry before.
	entryStart := make([]int, n)
	for i := 0; i < n; i++ {
		s := keyLine[i]
		floor := start
		if i > 0 {
			floor = keyLine[i-1] + 1
		}
		for s-1 >= floor && isCommentOrBlank(lines[s-1]) {
			s--
		}
		entryStart[i] = s
	}
	entryEnd := func(i int) int {
		if i+1 < n {
			return entryStart[i+1]
		}
		return end
	}

	freshVal := map[string]*yaml.Node{}
	var freshKeys []*yaml.Node
	for i := 0; i+1 < len(fresh.Content); i += 2 {
		freshVal[fresh.Content[i].Value] = fresh.Content[i+1]
		freshKeys = append(freshKeys, fresh.Content[i])
	}

	// A new key goes where the fresh encoding puts it: before the next key that
	// follows it there and is already in the file; after the last one else.
	oldKeys := map[string]bool{}
	for i := 0; i < n; i++ {
		oldKeys[old.Content[2*i].Value] = true
	}
	before := map[string][]*yaml.Node{}
	var trailing []*yaml.Node
	for fi, k := range freshKeys {
		if oldKeys[k.Value] {
			continue
		}
		anchor := ""
		for _, next := range freshKeys[fi+1:] {
			if oldKeys[next.Value] {
				anchor = next.Value
				break
			}
		}
		if anchor == "" {
			trailing = append(trailing, k)
		} else {
			before[anchor] = append(before[anchor], k)
		}
	}
	renderAdded := func(keys []*yaml.Node) ([]string, bool) {
		var r []string
		for _, k := range keys {
			if b, moved := blocks[scalarKey(freshVal[k.Value])]; moved {
				r = append(r, b.render(k.Value, indent)...) // a moved block keeps its lines
				continue
			}
			lines, ok := renderEntry(k, freshVal[k.Value], indent)
			if !ok {
				return nil, false
			}
			r = append(r, lines...)
		}
		return r, true
	}

	out := append([]string{}, lines[start:entryStart[0]]...)
	for i := 0; i < n; i++ {
		k, ov := old.Content[2*i], old.Content[2*i+1]
		s, e := entryStart[i], entryEnd(i)
		nv, kept := freshVal[k.Value]
		if !kept {
			continue // removed, with its head comments
		}
		if add := before[k.Value]; len(add) > 0 {
			r, ok := renderAdded(add)
			if !ok {
				return nil, false
			}
			out = append(out, r...)
		}
		switch {
		case sameYAMLValue(ov, nv):
			out = append(out, lines[s:e]...)
		case ov.Kind == yaml.MappingNode && nv.Kind == yaml.MappingNode && ov.Style&yaml.FlowStyle == 0 &&
			len(ov.Content) > 0 && ov.Line-1 > keyLine[i]:
			// Keep the key line (and its comment); splice the children.
			out = append(out, lines[s:keyLine[i]+1]...)
			body, tail := e, e
			for body-1 > keyLine[i] && isCommentOrBlank(lines[body-1]) {
				body-- // trailing blank lines stay after the spliced block
			}
			inner, ok := spliceMapping(lines, ov, nv, keyLine[i]+1, body, ov.Content[0].Column-1, blocks)
			if !ok {
				return nil, false
			}
			out = append(out, inner...)
			out = append(out, lines[body:tail]...)
		default:
			out = append(out, lines[s:keyLine[i]]...) // head comments
			r, ok := renderEntry(k, mergeYAMLNode(ov, nv), indent)
			if !ok {
				return nil, false
			}
			out = append(out, r...)
			// Blank lines that closed the old entry still close it.
			for j := e - 1; j > keyLine[i] && strings.TrimSpace(lines[j]) == ""; j-- {
				out = append(out, "")
			}
		}
	}
	// The rest go after the last entry, before the mapping's trailing blanks.
	added, ok := renderAdded(trailing)
	if !ok {
		return nil, false
	}
	if len(added) > 0 {
		cut := len(out)
		for cut > 0 && strings.TrimSpace(out[cut-1]) == "" {
			cut--
		}
		tail := append([]string{}, out[cut:]...)
		out = append(append(out[:cut], added...), tail...)
	}
	return out, true
}

// renderEntry encodes `key: value` at two-space indent, shifted to indent.
func renderEntry(key, value *yaml.Node, indent int) ([]string, bool) {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: key.Value, HeadComment: key.HeadComment, LineComment: key.LineComment},
		value,
	}}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if enc.Encode(m) != nil || enc.Close() != nil {
		return nil, false
	}
	pad := strings.Repeat(" ", indent)
	var out []string
	for _, l := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
		if l == "" {
			out = append(out, l)
			continue
		}
		out = append(out, pad+l)
	}
	return out, true
}

func isCommentOrBlank(l string) bool {
	t := strings.TrimSpace(l)
	return t == "" || strings.HasPrefix(t, "#")
}

// blockText is a block scalar as authored (`description: >-` and its folded
// lines), so a value that MOVES to another key keeps its folding.
type blockText struct {
	header string   // what follows `key:` on the key line — ` >-`
	body   []string // the continuation lines, their indent relative to the key
}

func (b blockText) render(key string, indent int) []string {
	pad := strings.Repeat(" ", indent)
	out := []string{pad + key + ":" + b.header}
	for _, l := range b.body {
		if strings.TrimSpace(l) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, pad+l)
	}
	return out
}

func scalarKey(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// blockScalars indexes every folded or literal scalar of the file by its value.
func blockScalars(lines []string, root *yaml.Node) map[string]blockText {
	out := map[string]blockText{}
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				if v.Kind == yaml.ScalarNode && (v.Style&(yaml.FoldedStyle|yaml.LiteralStyle)) != 0 && k.Line >= 1 && k.Line <= len(lines) {
					kl := k.Line - 1
					line := lines[kl]
					at := strings.Index(line, k.Value+":")
					if at < 0 {
						continue
					}
					b := blockText{header: line[at+len(k.Value)+1:]}
					col := k.Column - 1
					for j := kl + 1; j < len(lines); j++ {
						l := lines[j]
						if strings.TrimSpace(l) != "" && len(l)-len(strings.TrimLeft(l, " ")) <= col {
							break
						}
						if len(l) >= col {
							l = l[col:]
						}
						b.body = append(b.body, l)
					}
					for len(b.body) > 0 && strings.TrimSpace(b.body[len(b.body)-1]) == "" {
						b.body = b.body[:len(b.body)-1] // trailing blanks close the entry, not the value
					}
					if _, dup := out[v.Value]; !dup {
						out[v.Value] = b
					}
				}
				walk(v)
			}
		}
		for _, c := range n.Content {
			if n.Kind != yaml.MappingNode {
				walk(c)
			}
		}
	}
	walk(root)
	return out
}
