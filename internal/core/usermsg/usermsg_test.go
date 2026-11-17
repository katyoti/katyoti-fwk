// SPDX-License-Identifier: Apache-2.0

package usermsg

import (
	"errors"
	"fmt"
	"testing"
)

var errBase = errors.New("cwm: not found")

type typed struct{ id string }

func (t *typed) Error() string { return "typed " + t.id }
func (t *typed) UserMsg() (Msg, bool) {
	return Msg{Key: "x.typed", Params: map[string]any{"id": t.id}}, true
}

// A key attached keeps the text and the sentinel, and is found through any
// wrapping — fmt %w, a joined error, a typed carrier.
func TestOf_FindsTheKeyThroughTheChain(t *testing.T) {
	err := Wrap(fmt.Errorf("%w: object %q", errBase, "m.a"), "cwm.objectNotFound", map[string]any{"id": "m.a"})
	if err.Error() != `cwm: not found: object "m.a"` {
		t.Errorf("text changed: %q", err)
	}
	if !errors.Is(err, errBase) {
		t.Error("the sentinel is lost")
	}
	outer := fmt.Errorf("WorkspaceObjectGet: %w", err)
	if m, ok := Of(outer); !ok || m.Key != "cwm.objectNotFound" || m.Params["id"] != "m.a" {
		t.Errorf("Of = %+v %v", m, ok)
	}
	if !Is(errors.Join(errors.New("other"), &typed{"7"}), "x.typed") {
		t.Error("a typed carrier inside a joined error is not found")
	}
	if _, ok := Of(errors.New("plain")); ok {
		t.Error("a plain error has a key")
	}
	if Wrap(nil, "k", nil) != nil {
		t.Error("Wrap(nil) is not nil")
	}
}
