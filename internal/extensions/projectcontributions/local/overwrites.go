// SPDX-License-Identifier: Apache-2.0

package local

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	pcapi "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
)

// A project's overwrites/<provider>/ tree laid files over a member's packed
// content, file by file. It retires (decision): each
// entry it changes becomes an override object - an object carrying
// `contributes` with `overrides` and the layers the tree served - and the
// migration that runs this conversion writes those objects, then removes the
// tree. A file override widens to the whole layer it is read into, and the
// layers derived from the same file (the data a source carries) travel with
// it: each widening is reported. A file the conversion cannot map to an
// entry's layer keeps the tree: it is never retired while one remains.

// OverwritesConversion is what one provider's tree becomes.
type OverwritesConversion struct {
	Provider  string
	Overrides []pcapi.ConvertedOverride
	// Widenings say, per entry, which file overrides became which whole
	// layers.
	Widenings []string
	// Unmapped are the tree's files no entry's layer reads, with why when a
	// reader refused them.
	Unmapped []string
}

// Retirable reports whether the tree may be removed: every file of it maps
// to a layer of an override object.
func (c OverwritesConversion) Retirable() bool { return len(c.Unmapped) == 0 }

// ConvertOverwrites reads dir, a project's overwrites/<provider>/ tree laid
// out as the provider's pack (embed/<kind folder>/...), as that pack is read
// against kinds, and returns the override objects it becomes. Their layer
// files are placed under filesRoot (a folder of the module that will hold
// the objects): <filesRoot>/<kind>/<id>/<layer><ext>.
func ConvertOverwrites(ctx context.Context, dir, provider string, kinds contribapi.Kinds, filesRoot string) (OverwritesConversion, error) {
	out := OverwritesConversion{Provider: provider}
	ldr := pcapi.NewTreeLoader(dir)
	tree := ldr.Walk()
	mapped := map[string]bool{}  // tree files a written layer carries
	empty := map[string]string{} // tree files only an empty layer read
	packs, err := pcapi.PackLayerFiles(ctx, ldr, provider, kinds)
	if err != nil {
		return out, fmt.Errorf("overwrites %s: %w", provider, err)
	}
	for _, k := range packs {
		p := k.Provider
		for _, d := range k.Drops {
			out.Unmapped = append(out.Unmapped, d.Path+": "+d.Reason)
			mapped[d.Path] = true // named once, with its reason
		}
		for _, e := range k.Entries {
			conv, err := convertEntry(ctx, p, e, filesRoot)
			if err != nil {
				return out, fmt.Errorf("overwrites %s: %w", provider, err)
			}
			from := map[string]bool{}
			for layer, files := range k.Layers[conv.Overrides] {
				for _, f := range files.Files {
					if _, written := conv.Layers[layer]; written {
						from[f] = true
					} else {
						empty[f] = layer + " of " + conv.Overrides + " opened empty"
					}
				}
			}
			for f := range from {
				conv.From = append(conv.From, f)
				mapped[f] = true
			}
			sort.Strings(conv.From)
			out.Overrides = append(out.Overrides, conv)
			out.Widenings = append(out.Widenings, widening(conv))
		}
	}
	for _, f := range tree {
		if mapped[f] {
			continue
		}
		why := "no entry's layer reads it"
		if e, ok := empty[f]; ok {
			why = e
		}
		out.Unmapped = append(out.Unmapped, f+": "+why)
	}
	sort.Slice(out.Overrides, func(i, j int) bool { return out.Overrides[i].Overrides < out.Overrides[j].Overrides })
	sort.Strings(out.Widenings)
	sort.Strings(out.Unmapped)
	return out, nil
}

// convertEntry is the override object of one entry the tree serves: each of
// its layers opened and placed as a file.
func convertEntry(ctx context.Context, p contribapi.ContributionProvider, e contribapi.Contribution, filesRoot string) (pcapi.ConvertedOverride, error) {
	if e.Provider == "" {
		e.Provider = p.Provider()
	}
	if e.Type == "" {
		e.Type = p.Type()
	}
	c := pcapi.ConvertedOverride{Overrides: e.ResolvedURI(), Kind: e.Type, ID: e.ID, Layers: map[string]string{}, Files: map[string][]byte{}}
	names := make([]string, 0, len(e.Layers))
	for name := range e.Layers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rc, ref, err := p.Open(ctx, e.ID, name)
		if err != nil {
			return c, fmt.Errorf("%s: layer %s: %w", c.Overrides, name, err)
		}
		b, err := readAll(rc)
		if err != nil {
			return c, fmt.Errorf("%s: layer %s: %w", c.Overrides, name, err)
		}
		if t := bytes.TrimSpace(b); len(t) == 0 || string(t) == "null" {
			continue // opened empty (an empty YAML file reads as null): nothing to carry
		}
		file := pcapi.OverrideLayerFile(filesRoot, e.Type, e.ID, name, ref.MediaType)
		c.Layers[name] = "files/" + file
		c.Files[file] = b
	}
	return c, nil
}

// widening says which files became which whole layers.
func widening(c pcapi.ConvertedOverride) string {
	layers := make([]string, 0, len(c.Layers))
	for name := range c.Layers {
		layers = append(layers, name)
	}
	sort.Strings(layers)
	return fmt.Sprintf("%s: the file overrides %s become the whole layers %s (the entry's other layers are inherited)",
		c.Overrides, strings.Join(c.From, ", "), strings.Join(layers, ", "))
}

// readAll reads rc whole and closes it.
func readAll(rc io.ReadCloser) ([]byte, error) {
	defer rc.Close()
	return io.ReadAll(rc)
}
