// SPDX-License-Identifier: Apache-2.0

package render

import "testing"

// A page template holds a page tree the page grammar accepts
// an answer names it by its URI and SpecHash of its tree.
func TestPageTemplate(t *testing.T) {
	ok := PageTemplate{Label: "Étapes", Tree: map[string]any{"kind": "vbox", "id": "steps", "children": []any{map[string]any{"kind": "text", "content": "x"}}}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a valid template refused: %v", err)
	}
	spec, err := ok.Spec("ktw-x:/page-template/steps")
	want, _ := SpecHash(ok.Tree)
	if err != nil || spec.URI != "ktw-x:/page-template/steps" || spec.Hash != want {
		t.Errorf("its spec %+v (%v), want its URI and SpecHash of its tree %s", spec, err, want)
	}
	for name, bad := range map[string]PageTemplate{
		"no tree":         {Label: "x"},
		"an unknown kind": {Tree: map[string]any{"kind": "no-such-kind"}},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if !IsPageTemplateURI("ktw-x:/page-template/steps") || IsPageTemplateURI("ktw-x:/panel-type/steps") {
		t.Error("IsPageTemplateURI tells a page template's URI wrong")
	}
}
