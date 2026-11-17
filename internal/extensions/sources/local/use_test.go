// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : an object's « Sources » tab lists every contributed source
// connector - its own and one another member contributes, with no other
// change - each saying whether the object uses it; « Utiliser » creates the
// object's one source reading through it, and using it again answers that
// source.
func TestEveryContributedConnectorIsOfferedWithUtiliser(t *testing.T) {
	ctx, _, svc, _, _ := knowledgeWorld(t)

	uses := func() map[string]api.ConnectorUse {
		t.Helper()
		of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]api.ConnectorUse{}
		for _, c := range of.Connectors {
			out[c.Name] = c
		}
		return out
	}

	before := uses()
	for _, name := range []string{"local-folder", "wiki-page"} {
		c, ok := before[name]
		if !ok {
			t.Fatalf("the contributed connector %s is not offered: %v", name, before)
		}
		if c.Used || c.Source != "" {
			t.Fatalf("%s reads as used before any « Utiliser »: %+v", name, c)
		}
	}
	if before["wiki-page"].Label != "Wiki page" {
		t.Errorf("a connector is offered by its label: got %q", before["wiki-page"].Label)
	}

	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "wiki-page"})
	if err != nil {
		t.Fatal(err)
	}
	if !used.OK || !used.Created || used.Source == "" {
		t.Fatalf("« Utiliser » made no source: %+v", used)
	}
	after := uses()
	if c := after["wiki-page"]; !c.Used || c.Source != used.Source {
		t.Fatalf("wiki-page after « Utiliser » = %+v, want used by %s", c, used.Source)
	}
	if after["local-folder"].Used {
		t.Fatal("using one connector marked another as used")
	}

	again, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "wiki-page"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Created || again.Source != used.Source {
		t.Fatalf("using a connector twice made a second source: %+v, first %s", again, used.Source)
	}
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(of.Sources) != 1 || of.Sources[0].Connector != "wiki-page" {
		t.Fatalf("the object's sources = %+v, want the one wiki-page source", of.Sources)
	}

	if _, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "no-such-connector"}); err == nil {
		t.Fatal("a connector no member contributes was used")
	}
}

// The « Sources » tab offers the contributed connectors with « Utiliser »,
// in place of a create by connector name.
func TestTheSourcesTabOffersTheConnectors(t *testing.T) {
	raw, err := os.ReadFile("../contrib/embed/cwm/object-panels/sources.yaml")
	if err != nil {
		t.Fatal(err)
	}
	panel := string(raw)
	// Its board's switch is « Utiliser » on the row.
	for _, want := range []string{`kind: "board"`, `dataField: "$.connectors"`, `uri: "ktw-agents:sources/use"`, `connector: "${item.key}"`, `method: "ktw-agents:sources/connector-panel"`} {
		if !strings.Contains(panel, want) {
			t.Errorf("the « Sources » tab lacks %s", want)
		}
	}
	if strings.Contains(panel, `create: {type: "information-source"`) {
		t.Error("the « Sources » tab still creates a source by connector name")
	}
}
