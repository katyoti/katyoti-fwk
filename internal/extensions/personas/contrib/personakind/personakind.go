// SPDX-License-Identifier: Apache-2.0

// Package personakind is the `persona` kind as the personas pack declares it:
// its contribution-type (persona.yaml, the schema provider-relative) and its
// data-type, the shape of mbapi.PersonaBody. A leaf: it imports the leaf shape
// and the Go-type helper only, so the built-in kind reader
// (internal/core/contribution/builtin) can import it without a cycle.
package personakind

import (
	_ "embed"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed persona.yaml
var declaration []byte

// Leaf is the persona kind's declaration: its YAML and its schema. The schema
// is the shape of mbapi.PersonaBody (the task body plus the persona fields),
// a minor over the hand-written 1.1: every key 1.1 knew, plus the task and
// persona fields it left out (skills, tier, title, ...) — a 1.1 reader reads
// it, ignoring those.
func Leaf() leaf.Declaration {
	return leaf.Declaration{
		Kind: mbapi.ResourceTypePersona,
		YAML: declaration,
		Schema: manifestcontrib.GoTypeOf[mbapi.PersonaBody]("persona", "1.2.0", "Persona",
			"A reusable agent persona: an agent-loop task plus binding metadata\n"+
				"(how it attaches to an object, its default human-gate posture). Resolved\n"+
				"+ bound by CWM's persona library. Body extends the task shape with\n"+
				"persona-specific frontmatter.\n"),
	}
}
