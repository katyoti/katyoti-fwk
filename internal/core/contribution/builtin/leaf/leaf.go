// SPDX-License-Identifier: Apache-2.0

// Package leaf is the shape of a kind a pack declares itself: the one type a
// pack's kind leaf exposes and builtin reads. It imports nothing above the
// Go-type data-type helper, so a pack's leaf package stays a leaf.
package leaf

import (
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// Declaration is one kind its owning pack declares.
type Declaration struct {
	// Kind is the kind's name (the contribution-type id).
	Kind string
	// YAML is the embedded contribution-type declaration. Its schema is
	// provider-relative: whatever it states, the publisher names
	// `<provider>:/data-type/<Schema.ID>`.
	YAML []byte
	// Schema is the kind's data-type: the shape of the Go type the pack
	// decodes its entries into (manifestcontrib.GoTypeOf).
	Schema manifestcontrib.GoType
}
