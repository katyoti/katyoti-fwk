// SPDX-License-Identifier: Apache-2.0

// Package deckkind is the slide-deck kinds as the slide-deck pack declares
// them: `deck-theme`, `deck-asset` and `slide-group`, each a contribution-type
// (its YAML, the schema provider-relative, and a `source:` block saying how a
// pack's folder becomes entries) and a data-type, the shape of its deckbody
// type. A leaf: it imports the leaf shape, the Go-type helper and deckbody
// only, so the built-in kind reader (internal/core/contribution/builtin) can
// import it without a cycle — and a sidecar that ships themes or groups as
// folders of its pack never links slidedeck.
package deckkind

import (
	_ "embed"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api/deckbody"
)

// The kind names: the same as slidedeck/api's ContributionType constants,
// which this leaf cannot import.
const (
	KindDeckTheme  = "deck-theme"
	KindDeckAsset  = "deck-asset"
	KindSlideGroup = "slide-group"
)

var (
	//go:embed deck-theme.yaml
	themeYAML []byte
	//go:embed deck-asset.yaml
	assetYAML []byte
	//go:embed slide-group.yaml
	groupYAML []byte
)

// Theme is the deck-theme kind's declaration: its YAML and its schema.
func Theme() leaf.Declaration {
	return leaf.Declaration{
		Kind: KindDeckTheme,
		YAML: themeYAML,
		Schema: manifestcontrib.GoTypeOf[deckbody.ThemeBody](KindDeckTheme, "1.0.0", "Theme",
			"A deck theme: design tokens and fonts and the chrome every slide carries and\n"+
				"the looks of each layout and named icons. Chosen by a deck as `theme: <name>`.\n"),
	}
}

// Asset is the deck-asset kind's declaration: its YAML and its schema. The
// body is the file's path under deck-assets (a minor version above the
// {theme, path} body the theme producer wrote: the theme is the path's first
// segment).
func Asset() leaf.Declaration {
	return leaf.Declaration{
		Kind: KindDeckAsset,
		YAML: assetYAML,
		Schema: manifestcontrib.GoTypeOf[deckbody.AssetBody](KindDeckAsset, "1.1.0", "Asset",
			"One file a deck theme ships (a logo or a background or a font or an icon).\n"+
				"The file itself is the entry's source layer.\n"),
	}
}

// SlideGroup is the slide-group kind's declaration: its YAML and its schema.
func SlideGroup() leaf.Declaration {
	return leaf.Declaration{
		Kind: KindSlideGroup,
		YAML: groupYAML,
		Schema: manifestcontrib.GoTypeOf[deckbody.SlideGroupBody](KindSlideGroup, "1.0.0", "SlideGroup",
			"A slide group: slides a team keeps once and shows in many decks with a\n"+
				"slide whose only setting is include=<name>.\n"),
	}
}

// Leaves are the three deck kinds, in the order a binary publishes them.
func Leaves() []leaf.Declaration {
	return []leaf.Declaration{Theme(), Asset(), SlideGroup()}
}
