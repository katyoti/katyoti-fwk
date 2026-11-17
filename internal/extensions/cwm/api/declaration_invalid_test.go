// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// AN OBJECT TYPE THAT DOES NOT FIT ITS SCHEMA IS QUARANTINED.
// The catalog validates every kind as an error: an object type it marked
// invalid is not served to CWM - it stays named, so a refresh keeps the last
// good declaration in force - and an error finding shows the catalog's
// verdict. A valid one, and one not validated, are read.
func TestAnObjectTypeTheCatalogMarkedInvalidIsQuarantined(t *testing.T) {
	no, yes := false, true
	entry := func(id string, valid *bool, errs ...string) contribapi.Contribution {
		return contribapi.Contribution{Provider: "ktw-example", Type: "object-type", ID: id,
			Layers: contribapi.InlineData([]byte(`{"name":"` + id + `"}`)), Valid: valid, ValidationErrors: errs}
	}
	m := &mbapi.Manifest{Provider: "ktw-example", Contributions: []contribapi.Contribution{
		entry("broken", &no, "/icon: expected string"),
		entry("sound", &yes),
		entry("unchecked", nil),
	}}
	d := cwmapi.DeclarationsFromManifests([]*mbapi.Manifest{m}, nil)

	var names []string
	for _, o := range d.Objects {
		names = append(names, o.Name)
	}
	if strings.Join(names, ",") != "sound,unchecked" {
		t.Errorf("objects served = %v, want sound and unchecked only", names)
	}
	broken := m.Contributions[0].ResolvedURI()
	if !d.Named[broken] {
		t.Errorf("the quarantined type is not named: a refresh would drop its last good declaration")
	}
	if _, read := d.Sources[cwmapi.DeclarationKey("object-type", "broken")]; read {
		t.Error("the quarantined type is recorded as read")
	}
	if len(d.Findings) != 1 {
		t.Fatalf("findings = %+v, want one", d.Findings)
	}
	f := d.Findings[0]
	if f.Rule != cwmapi.FindingInvalid || f.Severity != cwmapi.FindingError || f.URI != broken ||
		!strings.Contains(f.Message, "/icon: expected string") {
		t.Errorf("finding = %+v, want an invalid error on %s carrying the catalog's verdict", f, broken)
	}
}
