// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// TestSourceBrowseWaitsForTheConnectorsBound: a caller of SourceBrowse (the
// GUI's « Parcourir ») waits at least as long as sources waits on the
// connector - with no declared timeout the bus gave it 5 s, and a Confluence
// space's root pages never came.
func TestSourceBrowseWaitsForTheConnectorsBound(t *testing.T) {
	def, ok := api.SourcesServiceDef.Methods["SourceBrowse"]
	if !ok {
		t.Fatal("SourceBrowse is not declared in SourcesServiceDef")
	}
	if got := time.Duration(def.TimeoutS) * time.Second; got <= browseTimeout {
		t.Fatalf("SourceBrowse's timeout %s does not outlast the connector's bound %s", got, browseTimeout)
	}
}
