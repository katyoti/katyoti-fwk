// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : the « Sources » tab carries a red badge counting the
// carrier's own sources that wait on the person - its last read failed (its
// error set, even while still declared) or it holds an item in failure -; a
// source only feeding it is not counted (it waits at its own carrier's tab),
// and a carrier whose sources all read shows none. The panel declares the
// count.
func TestTheSourcesTabCountsTheSourcesWaitingOnThePerson(t *testing.T) {
	ctx, cwm, svc, _, _ := knowledgeWorld(t)
	cwmapp.Module(t, ctx, cwm, "other")
	create := func(module, local, parent string, f api.InformationSourceFields) string {
		t.Helper()
		obj := cwmapi.NewContainerObject(module, local, api.InformationSourceType, parent, local, "")
		raw, _ := json.Marshal(f)
		obj.SetFieldsJSON(raw)
		out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: obj})
		if err != nil {
			t.Fatalf("create %s: %v", local, err)
		}
		return out.Object.ID
	}
	read := map[string]api.ItemState{"a": {State: api.ItemRead}}
	// m: one unreachable (error set, still declared), one holding a failed
	// item, one all read; a source of another module, in error, feeds it.
	create("m", "unreachable", "m", api.InformationSourceFields{Connector: "wiki-page", Error: "injoignable : 401"})
	// One source per connector per holder: each of m's reads its own.
	create("m", "one-failed", "m", api.InformationSourceFields{Connector: api.FolderConnector, States: map[string]api.ItemState{"a": {State: api.ItemRead}, "b": {State: api.ItemFailed, Why: "403"}}})
	create("m", "all-read", "m", api.InformationSourceFields{Connector: "maps", States: read})
	feeder := create("other", "feeder", "other", api.InformationSourceFields{Connector: "wiki-page", Error: "injoignable"})
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: feeder, To: "m", Kind: api.FeedsEdge}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.WaitingCount(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 2 || got.Color != "red" {
		t.Errorf("m's badge = %+v, want {2 red}: the unreachable one and the one with a failed item, never the feeding one", got)
	}

	// A carrier whose sources all read: no badge.
	cwmapp.Module(t, ctx, cwm, "calm")
	create("calm", "fine", "calm", api.InformationSourceFields{Connector: "wiki-page", States: read})
	create("calm", "never-read", "calm", api.InformationSourceFields{Connector: api.FolderConnector})
	if got, err = svc.WaitingCount(ctx, api.SourcesOfInput{ID: "calm"}); err != nil || got.Count != 0 || got.Color != "" {
		t.Errorf("calm's badge = %+v %v, want none", got, err)
	}

	// The panel declares it as its count.
	panel, err := os.ReadFile("../contrib/embed/cwm/object-panels/sources.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(panel), `count: "ktw-agents:sources/waiting-count"`) {
		t.Error("the Sources panel declares no count ktw-agents:sources/waiting-count")
	}
}
