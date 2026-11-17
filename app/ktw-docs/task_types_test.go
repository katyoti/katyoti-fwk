// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	actionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/actions/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	taskcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// « Création d'une présentation » and « Nouvelle trame » apply to the types
// where they make sense - a module, a document - never every object: the
// action surface of an information-source or a goal offers neither
// . Resolved on a real CWM through the action surface
// with the tasks as ktw-docs embeds them.
func TestPresentationAndTemplateApplyOnlyWhereTheyMakeSense(t *testing.T) {
	const deck, template = "ktw-docs:tasks/slidedeck-fast-create", "ktw-docs:tasks/content-template-new"
	var offered []actionsapi.TaskAction
	for _, name := range []string{"slidedeck-fast-create", "content-template-new"} {
		body, err := embeddedFS.ReadFile("embed/tasks/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		task, err := taskcontrib.Parse(body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		offered = append(offered, actionsapi.TaskAction{Provider: "ktw-docs", Name: task.Name, Label: task.Title,
			Kind: task.Kind, Types: task.Types, Filter: task.Filter, SessionMode: task.SessionMode})
	}
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":             {Data: []byte("name: module\ntree: node\n")},
			"object-types/document.yaml":           {Data: []byte("name: document\n")},
			"object-types/information-source.yaml": {Data: []byte("name: information-source\n")},
			"object-types/goal.yaml":               {Data: []byte("name: goal\n")},
		}),
		actionslocal.SurfaceExtensionWith(func(context.Context) []actionsapi.TaskAction { return offered }),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	for _, typ := range []string{"document", "information-source", "goal"} {
		o := cwmapi.Object{ID: "m." + typ, Type: typ}
		o.Name, o.Parent = typ, "m"
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", typ, err)
		}
	}
	surface, err := application.UseService[actionsapi.SurfaceService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"m": true, "m.document": true, "m.information-source": false, "m.goal": false} {
		out, err := surface.List(ctx, actionsapi.ListInput{ID: id})
		if err != nil {
			t.Fatalf("actions of %s: %v", id, err)
		}
		got := map[string]bool{}
		for _, a := range out.Actions {
			got[a.URI] = true
		}
		for _, uri := range []string{deck, template} {
			if got[uri] != want {
				t.Errorf("%s offers %s: %v, want %v", id, uri, got[uri], want)
			}
		}
	}
}
