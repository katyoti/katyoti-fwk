// SPDX-License-Identifier: Apache-2.0

package pptx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAML is the theme declaration as a theme folder holds it: the fields in
// the grammar's order, small maps (boxes, a look's tokens) on one line.
func (p *Proposal) YAML() ([]byte, error) {
	js, err := json.Marshal(p.Theme)
	if err != nil {
		return nil, err
	}
	// JSON is YAML: read into a node tree, which keeps the key order.
	var doc yaml.Node
	if err := yaml.Unmarshal(js, &doc); err != nil {
		return nil, err
	}
	restyle(&doc)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# Imported from a PowerPoint template by `deck import-theme`: review it\n# with its import report before use (theme %s).\n", p.Theme.Name)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), enc.Close()
}

// restyle drops JSON's quoting (the encoder quotes what needs it) and puts a
// mapping of a few scalars on one line.
func restyle(n *yaml.Node) {
	n.Style = 0
	for _, k := range n.Content {
		restyle(k)
	}
	if n.Kind == yaml.MappingNode && len(n.Content) <= 10 {
		for _, k := range n.Content {
			if k.Kind != yaml.ScalarNode {
				return
			}
		}
		n.Style = yaml.FlowStyle
	}
}

// Report is the review a person reads: the template, how each layout was
// mapped, and what the importer could not take or guessed.
func (p *Proposal) Report(inv *Inventory, source string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Import report — theme `%s`\n\n", p.Theme.Name)
	fmt.Fprintf(&b, "Source: `%s` — theme « %s », %s (%.0f × %.0f px), %d masters, %d layouts, %d example slides.\n\n",
		source, inv.Theme.Name, inv.Aspect, inv.Width, inv.Height, len(inv.Masters), len(inv.Layouts), len(inv.Slides))
	b.WriteString("## Layouts\n\n| Template layout | Part | Deck layout | Variant | By | Note |\n|---|---|---|---|---|---|\n")
	for _, m := range p.Mapping {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", m.Layout, m.Part, or(m.Deck, "—"), m.Variant, m.By, m.Note)
	}
	b.WriteString("\n## Colours\n\n| Token | Value |\n|---|---|\n")
	for _, k := range sortedKeys(p.Theme.Tokens) {
		if v := p.Theme.Tokens[k]; strings.HasPrefix(v, "#") {
			fmt.Fprintf(&b, "| %s | %s |\n", k, v)
		}
	}
	b.WriteString("\nThe template's scheme: ")
	var roles []string
	for _, r := range sortedKeys(inv.Theme.Colours) {
		roles = append(roles, fmt.Sprintf("%s %s", r, inv.Theme.Colours[r]))
	}
	b.WriteString(strings.Join(roles, ", ") + ".\n")
	b.WriteString("\n## Example slides\n\nThe text of the template's example slides, by layout: what the template says about writing in it.\n\n")
	byLayout := map[string]string{}
	for _, l := range inv.Layouts {
		byLayout[l.Part] = l.Name
	}
	for _, s := range inv.Slides {
		text := strings.Join(strings.Fields(s.Text), " ")
		if text == "" {
			continue
		}
		if r := []rune(text); len(r) > 400 {
			text = string(r[:400]) + "…"
		}
		fmt.Fprintf(&b, "- %s (%s): %s\n", path.Base(s.Part), or(byLayout[s.Layout], path.Base(s.Layout)), text)
	}
	b.WriteString("\n## To review\n\n")
	for _, n := range p.Notes {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	fmt.Fprintf(&b, "\n## Files\n\n")
	for _, f := range sortedKeys(p.Files) {
		fmt.Fprintf(&b, "- `%s/%s` (%d bytes)\n", p.Theme.Name, f, len(p.Files[f]))
	}
	return b.String()
}

// The pack folders a theme is written into: its declaration in
// deck-themes/<name>.yaml and its files under deck-assets/<name>/, the layout
// a sidecar's pack ships and the deck kinds read.
const (
	themesFolder = "deck-themes"
	assetsFolder = "deck-assets"
)

// Paths are where Write puts the theme under the pack root dir: its
// declaration and the folder of its files.
func (p *Proposal) Paths(dir string) (decl, files string) {
	return filepath.Join(dir, themesFolder, p.Theme.Name+".yaml"), filepath.Join(dir, assetsFolder, p.Theme.Name)
}

// Write writes the theme into the pack rooted at dir:
// <dir>/deck-themes/<name>.yaml and its files under <dir>/deck-assets/<name>/.
// It refuses to overwrite a theme already there.
func (p *Proposal) Write(dir string) error {
	decl, assets := p.Paths(dir)
	for _, existing := range []string{decl, assets} {
		if _, err := os.Stat(existing); err == nil {
			return fmt.Errorf("%s already exists: remove it or choose another --name", existing)
		}
	}
	y, err := p.YAML()
	if err != nil {
		return err
	}
	files := sortedKeys(p.Files)
	sort.Strings(files)
	for _, f := range files {
		target := filepath.Join(assets, filepath.FromSlash(path.Clean(f)))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, p.Files[f], 0o644); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(decl), 0o755); err != nil {
		return err
	}
	return os.WriteFile(decl, y, 0o644)
}
