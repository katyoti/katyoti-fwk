// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// : a drawer action opens either a node (checked as a page) or
// a panel; a stepper needs its steps.
func TestDrawerActionOpensANodeOrAPanel(t *testing.T) {
	text := render.Node{"kind": "text", "content": "x"}
	for name, c := range map[string]struct {
		action map[string]any
		ok     bool
	}{
		"a node":         {map[string]any{"drawer": map[string]any{"node": map[string]any(text)}}, true},
		"a panel":        {map[string]any{"drawer": map[string]any{"panel": "cadrage"}}, true},
		"both":           {map[string]any{"drawer": map[string]any{"node": map[string]any(text), "panel": "cadrage"}}, false},
		"neither":        {map[string]any{"drawer": map[string]any{"title": "t"}}, false},
		"a bad node":     {map[string]any{"drawer": map[string]any{"node": map[string]any{"kind": "nope"}}}, false},
		"with a uri too": {map[string]any{"drawer": map[string]any{"panel": "p"}, "uri": "a:b/c"}, false},
		// : a file of the object read in place.
		"a file":             {map[string]any{"drawer": map[string]any{"file": "as-is/01-contexte.md", "title": "Contexte"}}, true},
		"a file of another":  {map[string]any{"drawer": map[string]any{"file": "flux.drawio", "object": "${item.id}"}}, true},
		"a file and a panel": {map[string]any{"drawer": map[string]any{"file": "a.md", "panel": "cadrage"}}, false},
		"a file and a node":  {map[string]any{"drawer": map[string]any{"file": "a.md", "node": map[string]any(text)}}, false},
	} {
		err := render.CheckAction(c.action)
		if (err == nil) != c.ok {
			t.Errorf("%s: CheckAction = %v, want ok=%v", name, err, c.ok)
		}
	}
	a := render.Action{Drawer: &render.Drawer{Panel: "cadrage"}}
	if err := render.CheckAction(a); err != nil {
		t.Errorf("a typed drawer action: %v", err)
	}
}

func TestStepperIsAPageKindNeedingItsSteps(t *testing.T) {
	ok := map[string]any{"kind": "stepper", "steps": []any{map[string]any{"id": "a", "label": "A"}}, "current": "a"}
	if err := render.ValidatePage(ok); err != nil {
		t.Fatalf("a stepper: %v", err)
	}
	if err := render.ValidatePage(map[string]any{"kind": "stepper"}); err == nil || !strings.Contains(err.Error(), "steps") {
		t.Errorf("a stepper with no steps: %v", err)
	}
}
