// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : a type declares its icon, colour, plural label and lifecycle
// workflow; a project or module type the child types it holds in their tree
// order and its creation wizard; a project type the solution it needs and how
// it opens. A type with no icon is drawn with the default one, and a colour or
// a child order a renderer cannot draw is refused at declaration.
func TestATypeDeclaresWhatTellsItApart(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := stdctx.Background()

	dossier := api.ObjectType{
		Name: "dossier-architecture", Label: "Dossier d'architecture", LabelPlural: "Dossiers d'architecture",
		Icon: "folder", Color: "teal", Workflow: "manual", CreateTask: "ktw-acme:dossier/create",
		AllowedChildren: []string{"etude-existant", "documentation", "system-module"},
		ChildOrder:      []string{"etude-existant", "documentation"},
		Solution:        "archifactory", Opens: "ktw-acme:dossier/open",
	}
	svc.mergeObjectTypes([]api.ObjectType{
		dossier,
		{Name: "plain"},
		{Name: "bad-colour", Color: "not a colour!"},
		{Name: "twice", ChildOrder: []string{"a", "a"}},
		{Name: "outside", AllowedChildren: []string{"a"}, ChildOrder: []string{"b"}},
	}, nil)

	got, err := svc.ObjectTypeGet(ctx, api.ObjectTypeGetInput{Name: "dossier-architecture"})
	if err != nil {
		t.Fatal(err)
	}
	ot := got.ObjectType
	if ot.Color != "teal" || ot.Solution != "archifactory" || ot.Opens != "ktw-acme:dossier/open" ||
		ot.LabelPlural != "Dossiers d'architecture" || ot.Workflow != "manual" || ot.CreateTask == "" {
		t.Fatalf("the declared display facts did not read back: %+v", ot)
	}
	if ot.ChildRank("etude-existant") != 0 || ot.ChildRank("documentation") != 1 || ot.ChildRank("system-module") != 2 {
		t.Errorf("child order: etude-existant %d, documentation %d, system-module %d - want 0, 1, then after",
			ot.ChildRank("etude-existant"), ot.ChildRank("documentation"), ot.ChildRank("system-module"))
	}
	if ot.DisplayIcon() != "folder" {
		t.Errorf("a declared icon is drawn as declared, got %q", ot.DisplayIcon())
	}
	plain, ok := svc.lookupObjectType("plain")
	if !ok || plain.DisplayIcon() != api.DefaultTypeIcon {
		t.Errorf("a type with no icon is drawn with the default one, got %q", plain.DisplayIcon())
	}

	for _, name := range []string{"bad-colour", "twice", "outside"} {
		if _, ok := svc.lookupObjectType(name); ok {
			t.Errorf("the type %q has display facts no renderer can draw, yet it is registered", name)
		}
	}
}

func TestValidateDisplay(t *testing.T) {
	for _, c := range []string{"", "teal", "indigo", "#abc", "#a1b2c3", "dark-blue"} {
		if err := (api.ObjectType{Name: "x", Color: c}).ValidateDisplay(); err != nil {
			t.Errorf("colour %q refused: %v", c, err)
		}
	}
	for _, c := range []string{"Teal", "#abcd", "#ggg", "red;", "two words"} {
		if err := (api.ObjectType{Name: "x", Color: c}).ValidateDisplay(); err == nil {
			t.Errorf("colour %q accepted", c)
		}
	}
	if err := (api.ObjectType{Name: "x", ChildOrder: []string{"a", "b"}}).ValidateDisplay(); err != nil {
		t.Errorf("a child order with no allowed_children is free: %v", err)
	}
}
