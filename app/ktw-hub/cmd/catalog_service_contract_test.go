// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// The hub's implementation IS the CatalogService: every method the Go
// interface declares, the impl carries (and the wire publishes, since KTSP
// discovers methods on the concrete type). Before e14 the interface lacked
// OpenLayer while the Def and the wire had it; this line refuses the reverse
// drift at compile time. The interface == Def half is
// TestCatalogServiceInterfaceMatchesDef in manifest/api.
var _ mbapi.CatalogService = (*catalogServiceImpl)(nil)
