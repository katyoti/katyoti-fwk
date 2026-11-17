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

// : ktw-methods' goals framing reaches a chat on a
// goal and on any module - its type's module level capability, whatever the
// type's name - never on another object.
func TestTheGoalsFramingReachesAModuleOfAnotherTypeName(t *testing.T) {
	pack := fstest.MapFS{
		"capabilities/container.yaml":       {Data: []byte("name: container\n")},
		"object-types/software-module.yaml": {Data: []byte("name: software-module\ncapabilities: [module, container]\n")},
		"object-types/goal.yaml":            {Data: []byte("name: goal\ncapabilities: [container]\n")},
		"object-types/note.yaml":            {Data: []byte("name: note\ncapabilities: [container]\n")},
		"agent-contexts/goals.yaml":         {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/agent-contexts/goals.yaml")},
	}
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(pack), oclocal.ExtensionWith(api.ObjectContextConfig{}))
	for _, o := range []cwmapi.Object{
		{ID: "sw", Type: "software-module", Name: "Software"},
		{ID: "sw.g1", Type: "goal", Parent: "sw", Name: "Invoices go out on time"},
		{ID: "sw.n1", Type: "note", Parent: "sw", Name: "A note"},
	} {
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "sw", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	oc, err := application.UseService[api.ObjectContextService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	framed := func(id string) bool {
		t.Helper()
		out, err := oc.AgentContext(ctx, api.AgentContextInput{ID: id})
		if err != nil {
			t.Fatalf("AgentContext %s: %v", id, err)
		}
		for _, s := range out.System {
			if strings.HasPrefix(s.ID, "goals-") {
				return true
			}
		}
		return false
	}
	if !framed("sw") || !framed("sw.g1") {
		t.Errorf("the goals framing misses the software module or its goal")
	}
	if framed("sw.n1") {
		t.Errorf("the goals framing reaches a note")
	}
}
