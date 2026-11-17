// SPDX-License-Identifier: Apache-2.0

package api

import "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api/deckbody"

// ContributionTypeDeckTheme is the contribution kind of a theme declaration.
const ContributionTypeDeckTheme = "deck-theme"

// ContributionTypeDeckAsset is the contribution kind of one file a theme
// ships (a logo, a background, a font, an icon), published beside the theme
// as `<provider>:/deck-asset/<theme>/<path>`.
const ContributionTypeDeckAsset = "deck-asset"

// DefaultTheme is the theme of a deck that names none. Its values are the
// deck component's own defaults; its declaration overrides nothing.
const DefaultTheme = "katyoti"

// The theme shapes live in deckbody, which the deck kinds' leaf reads
// without importing this package; they keep their names here.
type (
	// ThemeBody is one theme declaration: the body of a deck-theme contribution.
	ThemeBody = deckbody.ThemeBody
	// ThemeColour is one colour of a theme's palette.
	ThemeColour = deckbody.ThemeColour
	// ThemeFont is one font file of a theme.
	ThemeFont = deckbody.ThemeFont
	// ThemeChrome is what every slide of a themed deck carries.
	ThemeChrome = deckbody.ThemeChrome
	// ThemeWatermark is what marks a theme's slides as produced.
	ThemeWatermark = deckbody.ThemeWatermark
	// ThemeImage is an image placed on the slide.
	ThemeImage = deckbody.ThemeImage
	// ThemeRule is a horizontal rule.
	ThemeRule = deckbody.ThemeRule
	// ThemeText is a line of text placed on the slide.
	ThemeText = deckbody.ThemeText
	// Box is a rectangle in the slide pixels of the theme's aspect.
	Box = deckbody.Box
	// ThemeLayout is how a theme treats one layout.
	ThemeLayout = deckbody.ThemeLayout
	// ThemeVariant is one look of a layout.
	ThemeVariant = deckbody.ThemeVariant
	// ThemeIcon is one pictogram of a theme.
	ThemeIcon = deckbody.ThemeIcon
)

// IsColour reports whether a background is a colour rather than a file.
func IsColour(background string) bool { return deckbody.IsColour(background) }
