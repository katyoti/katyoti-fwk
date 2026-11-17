// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	ocapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	oclocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// AN AGENT ON A RELEASE OR AN INCREMENT SEES ITS SCOPE — the shipped release and
// increment types give a chat on one the scope section (the work aimed at it,
// never the backlog) and the tool that reads that scope; a goal gets neither.
func TestAnAgentOnAReleaseOrAnIncrementSeesItsScope(t *testing.T) {
	t.Parallel()
	ship := func(p string) *fstest.MapFile {
		return &fstest.MapFile{Data: cwmapp.ShippedFile(t, "internal/extensions/planification/contrib/embed/cwm/"+p)}
	}
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"object-types/release.yaml":   ship("object-types/release.yaml"),
		"object-types/increment.yaml": ship("object-types/increment.yaml"),
		"capabilities/release.yaml":   ship("capabilities/release.yaml"),
		"workflows/release.yaml":      ship("workflows/release.yaml"),
		"workflows/increment.yaml":    ship("workflows/increment.yaml"),
		"object-types/goal.yaml":      {Data: []byte("name: goal\ncapabilities: [container]\n")},
	}), oclocal.Extension())
	cwmapp.Module(t, ctx, cwm, "billing")
	oc, err := application.UseService[ocapi.ObjectContextService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range []cwmapi.Object{
		{ID: "billing.RL-001", Type: "release", Name: "1.0"},
		{ID: "billing.IN-001", Type: "increment", Name: "Invoices reach customers"},
		{ID: "billing.G-001", Type: "goal", Name: "Customers pay on time"},
	} {
		o.Parent = "billing"
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	read := func(id string) string {
		t.Helper()
		out, err := oc.AgentContext(ctx, ocapi.AgentContextInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(out)
		return string(b)
	}
	for id, tool := range map[string]string{"billing.RL-001": "releases/scope", "billing.IN-001": "increments/scope"} {
		got := read(id)
		if !strings.Contains(got, "Work aimed at no release is backlog") || !strings.Contains(got, tool) {
			t.Errorf("%s's agent context lacks its scope section or %s: %s", id, tool, got)
		}
	}
	if got := read("billing.G-001"); strings.Contains(got, "backlog") || strings.Contains(got, "/scope") {
		t.Errorf("a goal's agent context carries a release's scope: %s", got)
	}
}
