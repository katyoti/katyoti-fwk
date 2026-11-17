// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// TestOneSourcePerConnectorPerHolder: a holder that asks for it - its type
// composes one-source-per-connector, as the as-is study v2 - reads a
// connector through one source. Created
// through the CWM, so through sources' before_create gate: under a v2 study a
// second source of a connector it already reads is refused « déjà dans
// l'étude »; another connector, the source written again and a source naming
// no connector are accepted; once archived (« Retirer ») a source frees its
// connector. A module and a v1 study carry no such capability: they keep two
// sources of one connector, as before the rule.
func TestOneSourcePerConnectorPerHolder(t *testing.T) {
	ctx, cwm, _, _ := sourcesWorld(t)
	create := func(parent, local, connector string) (string, error) {
		t.Helper()
		obj := cwmapi.NewContainerObject("m", local, api.InformationSourceType, parent, local, "")
		obj.Fields = map[string]any{"connector": connector}
		out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj})
		return out.Object.ID, err
	}
	holder := func(typ, local string) string {
		t.Helper()
		h := cwmapi.NewContainerObject("m", local, typ, "m", local, "")
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: h}); err != nil {
			t.Fatal(err)
		}
		return h.ID
	}

	// A module and a v1 study do not ask for it: two sources of a connector
	// are kept, as before the rule.
	if _, err := create("m", "folder-a", api.FolderConnector); err != nil {
		t.Fatalf("the module's first folder source is refused: %v", err)
	}
	if _, err := create("m", "folder-b", api.FolderConnector); err != nil {
		t.Errorf("a module's second folder source is refused - a module does not ask for one per connector: %v", err)
	}
	v1 := holder("study-v1", "ventes-v1")
	if _, err := create(v1, "v1-folder", api.FolderConnector); err != nil {
		t.Fatal(err)
	}
	if _, err := create(v1, "v1-folder-2", api.FolderConnector); err != nil {
		t.Errorf("a v1 study's second folder source is refused - v1 stays as it was: %v", err)
	}

	// A v2 study asks for one source per connector.
	v2 := holder("study-v2", "ventes")
	first, err := create(v2, "ventes-folder", api.FolderConnector)
	if err != nil {
		t.Fatalf("a v2 study's first folder source is refused: %v", err)
	}
	if _, err := create(v2, "ventes-folder-2", api.FolderConnector); err == nil || !strings.Contains(err.Error(), "ventes-folder est déjà dans l'étude") {
		t.Errorf("a v2 study's second folder source is not refused « déjà dans l'étude »: %v", err)
	}
	if _, err := create(v2, "ventes-wiki", "wiki-page"); err != nil {
		t.Errorf("a connector the v2 study does not read yet is refused: %v", err)
	}
	if _, err := create(v2, "ventes-bare", ""); err != nil {
		t.Errorf("a source naming no connector yet is refused: %v", err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: first})
	if err != nil {
		t.Fatal(err)
	}
	src := got.Object
	src.Description = "written again"
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: src}); err != nil {
		t.Errorf("the v2 study's own folder source written again is refused: %v", err)
	}

	// « Retirer » archives the source: its connector is free again.
	if _, err := cwm.ObjectArchive(ctx, cwmapi.ObjectArchiveInput{ID: first}); err != nil {
		t.Fatal(err)
	}
	if _, err := create(v2, "ventes-folder-3", api.FolderConnector); err != nil {
		t.Errorf("an archived source still holds its connector: %v", err)
	}
}

// TestTheOnePerConnectorRuleIsSourcesGate: the rule is held by the gate
// sources ships on its own type, before_create on information-source, and
// applies only under a holder carrying the capability sources declares.
func TestTheOnePerConnectorRuleIsSourcesGate(t *testing.T) {
	body := string(cwmapp.ShippedFile(t, "internal/extensions/sources/contrib/embed/cwm/hooks/source-check-on-create.yaml"))
	for _, want := range []string{"type: before_create", "uri: ktw-agents:sources/source-check", "types: [information-source]", "une seule source par connecteur", "one-source-per-connector"} {
		if !strings.Contains(body, want) {
			t.Errorf("the gate does not say %q:\n%s", want, body)
		}
	}
	capability := string(cwmapp.ShippedFile(t, "internal/extensions/sources/contrib/embed/cwm/capabilities/one-source-per-connector.yaml"))
	if !strings.Contains(capability, "name: "+api.OnePerConnectorCapability) {
		t.Errorf("sources does not declare the capability %s:\n%s", api.OnePerConnectorCapability, capability)
	}
}
