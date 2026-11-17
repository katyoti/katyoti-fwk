// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	displaytool "github.com/katyoti/katyoti-fwk/internal/extensions/tools/display_ui_component"
)

var defType = reflect.TypeOf(render.Def{})

// TestRenderDefIsTheOneShape — feature AC1. The LLM display tool
// EMBEDS render.Def, and an object panel's declared content and its resolved
// content ARE render.Def. A consumer growing its own copy of these fields is
// the drift this struct exists to end, so each is checked by type, not by
// field names that could be re-typed alike.
func TestRenderDefIsTheOneShape(t *testing.T) {
	embeds := func(t reflect.Type) bool {
		for i := 0; i < t.NumField(); i++ {
			if f := t.Field(i); f.Anonymous && f.Type == defType {
				return true
			}
		}
		return false
	}
	if !embeds(reflect.TypeOf(displaytool.DisplayUIComponentInput{})) {
		t.Error("the LLM display tool input must embed render.Def")
	}
	for name, typ := range map[string]reflect.Type{
		"PanelDecl.Content":   fieldType(t, reflect.TypeOf(paneapi.PanelDecl{}), "Content"),
		"ObjectPanel.Content": fieldType(t, reflect.TypeOf(paneapi.ObjectPanel{}), "Content"),
	} {
		if typ != reflect.PointerTo(defType) {
			t.Errorf("%s is %v, want *render.Def", name, typ)
		}
	}
	// The tool's own fields beyond the Def are the tool's: an artifact id,
	// action shortcuts, persistence. Anything else duplicates the Def.
	tool := reflect.TypeOf(displaytool.DisplayUIComponentInput{})
	for i := 0; i < tool.NumField(); i++ {
		f := tool.Field(i)
		if f.Anonymous {
			continue
		}
		switch f.Name {
		case "ArtifactID", "Actions", "Persistent":
		default:
			t.Errorf("the tool declares %s beside the embedded Def — a display field belongs in render.Def", f.Name)
		}
	}
}

func fieldType(t *testing.T, typ reflect.Type, name string) reflect.Type {
	t.Helper()
	f, ok := typ.FieldByName(name)
	if !ok {
		t.Fatalf("%s has no field %s", typ, name)
	}
	return f.Type
}
