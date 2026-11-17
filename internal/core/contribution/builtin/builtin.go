// SPDX-License-Identifier: Apache-2.0

// Package builtin is the one reader of the kinds compiled into a binary: the
// system set and the kinds their owning packs declare (leaf packages it
// imports, each a leaf.Declaration: skill, task, persona, source-connector,
// publication-connector, the drawio kinds notation, viewpoint and
// drawio-template, and the slide-deck kinds deck-theme, deck-asset and
// slide-group). A reader that must know a
// kind without holding a manifest — a lint of a file on disk, a host wiring a
// folder — asks Kinds() / Formats() here, never contribution.SystemKinds():
// a kind that leaves the system set for its pack stays known to every reader.
//
// ContributeLeafKinds publishes a binary's leaf kinds, once per binary.
package builtin

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	drawiokind "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/contrib/drawiokind"
	personakind "github.com/katyoti/katyoti-fwk/internal/extensions/personas/contrib/personakind"
	publisherkind "github.com/katyoti/katyoti-fwk/internal/extensions/publications/contrib/publisherkind"
	skillkind "github.com/katyoti/katyoti-fwk/internal/extensions/skills/contrib/skillkind"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib/deckkind"
	connectorkind "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib/connectorkind"
	taskkind "github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib/taskkind"
)

// ReaderProvider is the provider Kinds() names a leaf's declaration and
// schema under: a reader with no binary identity. A publication names the
// publishing provider instead (ContributeLeafKinds).
const ReaderProvider = "builtin"

// leaves is every pack-owned kind leaf this package imports: the kinds whose
// home is their pack, not the system set.
var leaves = append([]leaf.Declaration{skillkind.Leaf(), taskkind.Leaf(), personakind.Leaf(), connectorkind.Leaf(),
	publisherkind.Leaf()}, append(drawiokind.Leaves(), deckkind.Leaves()...)...)

var (
	kindsOnce sync.Once
	kinds     contribapi.Kinds
	kindsErr  error

	formatsOnce sync.Once
	formats     map[string]string
)

// Leaves lists the pack-owned kind leaves this binary carries, the kinds
// Kinds() takes from their pack: what a binary's
// bootstrap hands ContributeLeafKinds. Named kinds keep only those (a name no
// leaf declares is none); no name lists them all. The caller owns the copy.
func Leaves(kinds ...string) []leaf.Declaration {
	if len(kinds) == 0 {
		return append([]leaf.Declaration(nil), leaves...)
	}
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	var out []leaf.Declaration
	for _, l := range leaves {
		if want[l.Kind] {
			out = append(out, l)
		}
	}
	return out
}

// Kinds is the kind registry of every kind compiled into this binary: the
// system set and the leaves. Computed once.
func Kinds() (contribapi.Kinds, error) {
	kindsOnce.Do(func() { kinds, kindsErr = kindsOf(leaves) })
	return kinds, kindsErr
}

// Formats is the format (data-type major.minor) of every kind Kinds()
// declares — what a host built from this binary reads (contribapi.JudgeFormat).
// Computed once; the caller owns the copy.
func Formats() map[string]string {
	formatsOnce.Do(func() { formats = formatsOf(leaves) })
	out := make(map[string]string, len(formats))
	for k, v := range formats {
		out[k] = v
	}
	return out
}

// kindsOf is the system declarations plus ls. A leaf naming a system kind is
// refused: a kind has one home, its pack or the system set, never both.
func kindsOf(ls []leaf.Declaration) (contribapi.Kinds, error) {
	sys, err := contribution.SystemKindEntries()
	if err != nil {
		return contribapi.Kinds{}, err
	}
	declared := contribapi.KindsFrom(sys)
	entries := make([]contribapi.Contribution, 0, len(sys)+len(ls))
	entries = append(entries, sys...)
	for _, l := range ls {
		if declared.DeclaredBy(l.Kind) != "" {
			return contribapi.Kinds{}, fmt.Errorf("builtin kind %s: declared by its pack and by the system set (%s)", l.Kind, declared.DeclaredBy(l.Kind))
		}
		e, err := entryOf(ReaderProvider, l)
		if err != nil {
			return contribapi.Kinds{}, err
		}
		entries = append(entries, e)
	}
	return contribapi.KindsFrom(entries), nil
}

// formatsOf is the system formats plus the formats of ls's schemas.
func formatsOf(ls []leaf.Declaration) map[string]string {
	out := contribution.SystemFormats()
	for _, l := range ls {
		doc, err := l.Schema.Document()
		if err != nil {
			continue
		}
		if f := contribapi.DataTypeFormat(doc); f != "" {
			out[l.Kind] = f
		}
	}
	return out
}

// entryOf is a leaf's contribution-type entry under provider: its YAML as the
// data layer (JSON), the schema named under provider, and the YAML as written
// as the source layer.
func entryOf(provider string, l leaf.Declaration) (contribapi.Contribution, error) {
	var tree map[string]any
	// Content, not configuration: decoded with no variable expansion.
	if err := marshal.Unmarshal(l.YAML, marshal.YAML, &tree); err != nil {
		return contribapi.Contribution{}, fmt.Errorf("builtin kind %s: yaml parse: %w", l.Kind, err)
	}
	if tree == nil {
		tree = map[string]any{}
	}
	if n, _ := tree["name"].(string); n == "" {
		tree["name"] = l.Kind
	} else if n != l.Kind {
		return contribapi.Contribution{}, fmt.Errorf("builtin kind %s: its declaration names %q", l.Kind, n)
	}
	tree["schema"] = contribapi.Contribution{Provider: provider, Type: contribapi.ResourceTypeDataType, ID: l.Schema.ID}.ResolvedURI()
	body, err := json.Marshal(tree)
	if err != nil {
		return contribapi.Contribution{}, fmt.Errorf("builtin kind %s: json: %w", l.Kind, err)
	}
	e := contribapi.Contribution{
		Provider: provider,
		Type:     contribapi.ResourceTypeContributionType,
		ID:       l.Kind,
		Metadata: &contribapi.ContributionMeta{MediaType: "application/json"},
	}
	e.SetData(body)
	e.SetSource(l.YAML, "application/yaml")
	return e, nil
}
