// SPDX-License-Identifier: Apache-2.0

package api

// ImportThemeInput asks the DeckService to turn a PowerPoint template into a
// deck theme (`ktw-docs:deck/import-theme`): a dry run that reports the
// proposal; with Module, a deck-theme object of that module (its files and
// the template in themes/<name>/); with Folder, the pack root a sidecar
// ships (deck-themes/<name>.yaml and deck-assets/<name>/).
type ImportThemeInput struct {
	Template    string          `json:"template"              jsonschema:"description=Path of the PowerPoint template (.pptx or .potx); a relative path is the workspace's"`
	Name        string          `json:"name"                  jsonschema:"description=The theme's name (kebab-case ASCII) as a deck's theme: setting names it"`
	Description string          `json:"description,omitempty" jsonschema:"description=The theme's one-line description; the template's theme name when empty"`
	Decisions   ImportDecisions `json:"decisions,omitempty"   jsonschema:"description=What the importer cannot decide alone: applied over its own proposal"`
	Module      string          `json:"module,omitempty"      jsonschema:"description=The module the theme is stored in as a deck-theme object (its files and the template in themes/<name>/); decks of the project can use it at once"`
	Folder      string          `json:"folder,omitempty"      jsonschema:"description=The pack root the theme is written into (<folder>/deck-themes/<name>.yaml and <folder>/deck-assets/<name>/); empty for a dry run that writes nothing"`
}

// ImportDecisions are what a person or an agent decides over the importer's
// proposal, read from its report.
type ImportDecisions struct {
	// Layouts maps a template layout — its name, or its part
	// (slideLayout7.xml) when two layouts share a name — to the deck layout
	// it becomes, or `drop`.
	Layouts map[string]string `json:"layouts,omitempty" jsonschema:"description=Template layout (its name or its part such as slideLayout7.xml) to the deck layout it becomes or drop"`
	// Variants renames a proposed variant: `cover/sand` → `sable`.
	Variants map[string]string `json:"variants,omitempty" jsonschema:"description=A proposed variant as <layout>/<variant> to its new kebab-case name"`
	// Guidance is the theme's house rules for authors (markdown).
	Guidance string `json:"guidance,omitempty" jsonschema:"description=The theme's house rules for deck authors (markdown)"`
}

// ImportThemeOutput is the importer's proposal and what it wrote.
type ImportThemeOutput struct {
	Report      string          `json:"report"            jsonschema:"description=The import report (markdown): the mapping and the colours and what to review"`
	Mapping     []ImportMapping `json:"mapping"           jsonschema:"description=What each template layout became"`
	Notes       []string        `json:"notes,omitempty"   jsonschema:"description=What a person reviews: guesses and what was not imported"`
	Declaration string          `json:"declaration"       jsonschema:"description=The theme declaration (YAML) as written or as it would be"`
	Object      string          `json:"object,omitempty"  jsonschema:"description=The deck-theme object created when a module was given"`
	Written     []string        `json:"written,omitempty" jsonschema:"description=The files written; empty for a dry run"`
}

// ImportMapping is what one template layout became.
type ImportMapping struct {
	Layout  string `json:"layout"            jsonschema:"description=The template layout's name"`
	Part    string `json:"part"              jsonschema:"description=Its part in the template such as slideLayout7.xml"`
	Deck    string `json:"deck,omitempty"    jsonschema:"description=The deck layout it became; empty when none"`
	Variant string `json:"variant,omitempty" jsonschema:"description=The variant it became"`
	By      string `json:"by"                jsonschema:"description=How it was decided,enum=name|structure|decision|none"`
	Note    string `json:"note,omitempty"    jsonschema:"description=Why or what to review"`
}
