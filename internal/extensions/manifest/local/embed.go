// SPDX-License-Identifier: Apache-2.0

package local

import "embed"

// dataTypesFS embeds the canonical ReferenceBody + ConfigBody DSL
// Schemas shipped by this extension. Any binary importing
// `manifestlocal.Extension()` gets the data-type Contributions emitted
// automatically. The hub's catalog Phase-2 wire-up registers them
// with SchemaService at boot.
//
//go:embed embed/data-types
var dataTypesFS embed.FS
