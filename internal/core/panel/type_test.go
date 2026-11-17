// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"testing"
	"time"
)

// A panel type gives its page tree by URI or inline - one of
// them -, the inputs it takes, how long it is kept and what stays.
func TestAPanelTypeIsValidatedAsItsKindSays(t *testing.T) {
	ok := TypeDecl{Page: "ktw-x:/page-template/steps", Inputs: []string{"submit", "cancel"}, Retention: "7d", Keep: KeepOutcome}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a valid type refused: %v", err)
	}
	if d, _ := ok.RetentionDuration(); d != 7*24*time.Hour {
		t.Errorf("7d is %v", d)
	}
	if d, err := (TypeDecl{Retention: "36h"}).RetentionDuration(); err != nil || d != 36*time.Hour {
		t.Errorf("36h is %v %v", d, err)
	}
	if (TypeDecl{Tree: map[string]any{"kind": "text"}}).KeepOrDefault() != KeepNone {
		t.Error("a type saying nothing keeps nothing")
	}
	for name, bad := range map[string]TypeDecl{
		"both a page and a tree": {Page: "x:/page-template/a", Tree: map[string]any{"kind": "text"}},
		"neither":                {},
		"an unknown input":       {Page: "x:/page-template/a", Inputs: []string{"shout"}},
		"a retention unread":     {Page: "x:/page-template/a", Retention: "a week"},
		"an unknown keep":        {Page: "x:/page-template/a", Keep: "everything"},
	} {
		if bad.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
