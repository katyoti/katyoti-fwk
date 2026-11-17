// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// The panel lists the members' entries the project overrides, one row per
// entry with the layers it serves of it, entries and layers sorted; the
// project's overrides are those its tree carries in the manifests.
func TestTheOverriddenEntriesAreGroupedByEntry(t *testing.T) {
	got := OverriddenEntries([]contribapi.Override{
		{URI: "ktw-compose:/skill/greet", Layer: "source", From: "project:acme"},
		{URI: "ktw-agents:/task/review", Layer: "translations", From: "project:acme"},
		{URI: "ktw-compose:/skill/greet", Layer: "instructions", From: "project:acme"},
	})
	if len(got) != 2 || got[0].URI != "ktw-agents:/task/review" || got[1].URI != "ktw-compose:/skill/greet" {
		t.Fatalf("entries = %+v", got)
	}
	if strings.Join(got[1].Layers, ",") != "instructions,source" {
		t.Fatalf("greet's layers = %v", got[1].Layers)
	}
	if contribapi.ProjectOrigin("acme") != "project:acme" {
		t.Fatalf("origin = %q", contribapi.ProjectOrigin("acme"))
	}
}
