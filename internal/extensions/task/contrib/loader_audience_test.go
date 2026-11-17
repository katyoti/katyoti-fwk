// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/json"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A task declares whom it is for: audience
// end-user | technical | demo is task metadata beside kind and types, never
// config; an unknown value is refused; an undeclared task has none, which no
// reader takes for end-user. The catalog body reads the same key.
func TestParse_Audience(t *testing.T) {
	got, err := Parse([]byte(`---
name: deck-theme-import
type: agent
audience: end-user
---
body`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Audience != api.AudienceEndUser {
		t.Errorf("Audience = %q, want %q", got.Audience, api.AudienceEndUser)
	}
	if _, inConfig := got.Config["audience"]; inConfig {
		t.Error("audience leaked into config; it is task metadata")
	}

	undeclared, err := Parse([]byte(`---
name: worker
type: agent
---
body`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if undeclared.Audience != "" {
		t.Errorf("an undeclared task has audience %q, want none", undeclared.Audience)
	}

	if _, err := Parse([]byte(`---
name: bogus
type: agent
audience: everyone
---
body`)); err == nil {
		t.Error("audience 'everyone' accepted; want refused (end-user | technical | demo)")
	}
	if _, err := Parse([]byte(`---
name: bogus
type: agent
audience: [end-user]
---
body`)); err == nil {
		t.Error("a list as audience accepted; want refused")
	}

	var body mbapi.TaskBody
	if err := json.Unmarshal([]byte(`{"name":"deck-theme-import","type":"agent","audience":"end-user"}`), &body); err != nil {
		t.Fatal(err)
	}
	if body.Audience != api.AudienceEndUser {
		t.Errorf("the catalog body's audience = %q, want %q", body.Audience, api.AudienceEndUser)
	}
	// The lists read through the catalog (the GUI's gallery and pickers).
	if got := api.TaskSummaryFromCatalog("ktw-docs", body).Audience; got != api.AudienceEndUser {
		t.Errorf("the catalog summary's audience = %q, want %q", got, api.AudienceEndUser)
	}
}
