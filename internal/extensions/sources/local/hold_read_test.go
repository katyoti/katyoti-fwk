// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : a source's picker « Enregistrer et lire » keeps the choice
// and reads it at once - the items read, the message saying how many; an
// item that cannot be read says why and what reads it later, never « pas
// encore lus : ils le seront quand vous appliquerez ».
func TestThePickersSaveReadsWhatItKeeps(t *testing.T) {
	pickVersion = "v1"
	defer func() { pickVersion = "v1" }()
	ctx, _, svc, _ := pickWorld(t)

	out, err := svc.Hold(ctx, api.HoldInput{Object: "m", Connector: "apps", Read: true,
		Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}, {Key: "A2", Title: "Acme Stock"}}})
	if err != nil || !out.OK {
		t.Fatalf("save and read: %v %+v", err, out)
	}
	if out.Result == nil || strings.Join(out.Result.Read, ",") != "A1,A2" {
		t.Fatalf("the kept items are read: %+v", out.Result)
	}
	if !strings.HasPrefix(out.Message, "2 élément(s) lu(s).") || strings.Contains(out.Message, "pas encore lus") {
		t.Errorf("the message says what was read: %q", out.Message)
	}
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil || len(of.Sources) != 1 {
		t.Fatalf("sources: %v %+v", err, of.Sources)
	}
	st := of.Sources[0].States
	if st["A1"].State != api.ItemRead || st["A2"].State != api.ItemRead || of.Sources[0].Held != 0 {
		t.Errorf("read, nothing left held: %+v held=%d", st, of.Sources[0].Held)
	}

	// An item that cannot be read: the message says why and what reads it.
	out, err = svc.Hold(ctx, api.HoldInput{Object: "m", Connector: "apps", Read: true,
		Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}, {Key: "BAD", Title: "Gone"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.Message, "1 élément(s) lu(s). 1 non lu(s) : ") || !strings.Contains(out.Message, "not found") ||
		!strings.Contains(out.Message, "la prochaine synchronisation de la source les lit") {
		t.Errorf("why it was not read and what reads it: %q", out.Message)
	}

	// Without read, holding keeps the old behaviour: nothing is read.
	out, err = svc.Hold(ctx, api.HoldInput{Object: "m", Connector: "apps", Items: []api.SelectedItem{{Key: "A3", Title: "Acme Paie"}}})
	if err != nil || out.Result != nil || !strings.Contains(out.Message, "pas encore lus") {
		t.Errorf("a plain hold reads nothing: %v %+v", err, out)
	}
}

// : the picker's action is the mock-up's « Enregistrer et
// lire », holding with read.
func TestThePickersActionIsSaveAndRead(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	if _, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "apps"}); err != nil {
		t.Fatal(err)
	}
	p, err := svc.ConnectorPanel(ctx, api.ConnectorPanelInput{ID: "m", Connector: "apps"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p.DSLTree)
	s := string(raw)
	for _, want := range []string{`"apply":"Enregistrer et lire"`, `"read":true`, `"apply":"ktw-agents:sources/hold"`} {
		if !strings.Contains(s, want) {
			t.Errorf("picker misses %s: %s", want, s)
		}
	}
}
