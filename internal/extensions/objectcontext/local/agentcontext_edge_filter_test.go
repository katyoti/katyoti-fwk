// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	oclocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// AN AGENT CONTEXT BY EDGE: an `agent-context` whose filter says
// `incoming: [challenges]` frames a chat on the object a challenge points at,
// and not on one nothing challenges; `outgoing:` frames the challenger. A
// framing whose filter carries a key no field takes is skipped alone.
func TestAnAgentContextFilterSelectsByEdge(t *testing.T) {
	pack := fstest.MapFS{
		"object-types/module.yaml":       {Data: []byte("name: module\n")},
		"object-types/thing.yaml":        {Data: []byte("name: thing\n")},
		"edge-types/challenges.yaml":     {Data: []byte("name: challenges\nfrom: any\nto: any\ncardinality: M:N\ntarget: object\nobject_storage: edges\n")},
		"agent-contexts/challenged.yaml": {Data: []byte("name: challenged\nfilter: {incoming: [challenges]}\nagent:\n  system:\n    - {id: answer-it, text: \"this object is challenged\"}\n")},
		"agent-contexts/challenger.yaml": {Data: []byte("name: challenger\nfilter: {outgoing: [challenges]}\nagent:\n  system:\n    - {id: argue-it, text: \"this object challenges\"}\n")},
		"agent-contexts/misspelt.yaml":   {Data: []byte("name: misspelt\nfilter: {incomming: [challenges]}\nagent:\n  system:\n    - {id: never, text: \"a misspelt filter\"}\n")},
	}
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(pack), oclocal.ExtensionWith(api.ObjectContextConfig{}))
	cwmapp.Module(t, ctx, cwm, "m")
	for _, o := range []cwmapi.Object{
		{ID: "m.target", Type: "thing"},
		{ID: "m.lone", Type: "thing"},
		{ID: "m.c", Type: "thing", Edges: []cwmapi.ObjectEdge{{Kind: "challenges", To: "m.target"}}},
	} {
		o.Parent, o.Name = "m", o.ID
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	oc, err := application.UseService[api.ObjectContextService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id, want, not string
	}{
		{"m.target", "answer-it@", "argue-it@"},
		{"m.c", "argue-it@", "answer-it@"},
		{"m.lone", "", "answer-it@"},
	} {
		got := sectionIDs(agentContext(t, ctx, oc, c.id))
		if c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: sections %q must hold %s", c.id, got, c.want)
		}
		if strings.Contains(got, c.not) || strings.Contains(got, "never@") {
			t.Errorf("%s: sections %q must not hold %s (nor the misspelt framing)", c.id, got, c.not)
		}
	}
}
