// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestAPersonsActIsAskedByItsCode: a person's act names its
// permission code; a person may do it, an agent or no actor is refused with
// the code in the refusal, and a malformed code is refused whoever asks.
func TestAPersonsActIsAskedByItsCode(t *testing.T) {
	code := HumanTaskCode("release", "approve")
	if err := PersonAct(cwmapi.Actor{ID: "jdoe", Kind: "person"}, code); err != nil {
		t.Errorf("a person's act by a person: %v", err)
	}
	for _, by := range []cwmapi.Actor{{ID: "claude-x", Kind: "agent"}, {}} {
		err := PersonAct(by, code)
		if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), string(code)) {
			t.Errorf("a person's act by %+v: %v; want refused naming %s", by, err, code)
		}
	}
	if err := PersonAct(cwmapi.Actor{ID: "jdoe", Kind: "person"}, "Not A Code"); err == nil {
		t.Error("a malformed code is accepted")
	}
	if !IsPerson(cwmapi.Actor{Kind: "person"}) || IsPerson(cwmapi.Actor{Kind: "agent"}) {
		t.Error("IsPerson reads the actor's kind wrong")
	}
}
