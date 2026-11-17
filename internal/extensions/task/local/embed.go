// SPDX-License-Identifier: Apache-2.0

package local

import "embed"

// dataTypesFS embeds the canonical TaskBody DSL Schema shipped by this
// extension. Any binary importing `tasklocal.Extension()` gets the
// data-type Contribution emitted automatically via the manifest's
// `ContributeDataTypeResources`. The hub's catalog Phase-2 wire-up
// then registers it with SchemaService at boot.
//
//go:embed embed/data-types
var dataTypesFS embed.FS
